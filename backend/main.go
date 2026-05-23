
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/time/rate"
	
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

type User struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type Task struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	ID      int    `json:"id"`
	Status  string `json:"status"`
}

type Log struct {
	ID     int    `json:"id"`
	TaskID int    `json:"task_id"`
	Output string `json:"output"`
	Status string `json:"status"`
}

var db *sql.DB
var rdb *redis.Client
var ctx = context.Background()
var (
	requestCount = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			  Help: "Total HTTP requests",
        },
        []string{"method", "path"},
    )
	requestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "http_request_duration_seconds",
            Help: "HTTP request duration",
        },
        []string{"path"},
    )
)

func initDB() {
	connStr := "user=postgres dbname=automation sslmode=disable"
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		slog.Error("db connection error", "error", err)
	}
	slog.Info("db connected")
}

func initRedis() {
	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	slog.Info("redis connected")
}

func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w)
		if r.Method == "OPTIONS" {
			return
		}
		tokenString := r.Header.Get("Authorization")
		if tokenString == "" {
			fmt.Fprintln(w, "no token provided")
			return
		}
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			return []byte("secret_key"), nil
		})
		if err != nil || !token.Valid {
			fmt.Fprintln(w, "invalid token")
			return
		}
		next(w, r)
	}
}

var limiter = rate.NewLimiter(10, 10)

func rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

func handleRegister(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method != "POST" {
		fmt.Fprintln(w, "method not allowed")
		return
	}
	var user User
	json.NewDecoder(r.Body).Decode(&user)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), 14)
	if err != nil {
		fmt.Fprintln(w, "hash error:", err)
		return
	}
	_, err = db.Exec(
		"INSERT INTO users (email, password) VALUES ($1, $2)",
		user.Email, hashedPassword,
	)
	if err != nil {
		fmt.Fprintln(w, "db error:", err)
		return
	}
	fmt.Fprint(w, `{"status": "registered"}`)
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method != "POST" {
		fmt.Fprintln(w, "method not allowed")
		return
	}
	var user User
	json.NewDecoder(r.Body).Decode(&user)

	var storedPassword string
	var userID int
	err := db.QueryRow("SELECT id, password FROM users WHERE email = $1",
		user.Email).Scan(&userID, &storedPassword)
	if err != nil {
		fmt.Fprintln(w, "user not found")
		return
	}
	err = bcrypt.CompareHashAndPassword([]byte(storedPassword), []byte(user.Password))
	if err != nil {
		fmt.Fprintln(w, "wrong password")
		return
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id": userID,
		"exp":     time.Now().Add(24 * time.Hour).Unix(),
	})
	tokenString, _ := token.SignedString([]byte("secret_key"))
	fmt.Fprint(w, tokenString)
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method != "GET" {
		fmt.Fprintln(w, "method not allowed")
		return
	}
	id := r.URL.Path[len("/logs/"):]
	rows, err := db.Query("SELECT id, task_id, output, status FROM job_logs WHERE task_id = $1", id)
	if err != nil {
		fmt.Fprintln(w, "db error:", err)
		return
	}
	var allLogs []Log
	for rows.Next() {
		var l Log
		rows.Scan(&l.ID, &l.TaskID, &l.Output, &l.Status)
		allLogs = append(allLogs, l)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(allLogs)
}

func handleExecute(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method != "POST" {
		fmt.Fprintln(w, "method not allowed")
		return
	}
	id := r.URL.Path[len("/execute/"):]
	err := rdb.LPush(ctx, "job_queue", id).Err()
	if err != nil {
		fmt.Fprintln(w, "redis error:", err)
		return
	}
	fmt.Fprintln(w, "job queued! task id:", id)
}

func handleTasks(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")
		rows, err := db.Query("SELECT id, name, command, status FROM tasks")
		if err != nil {
			fmt.Fprintln(w, "db error:", err)
			return
		}
		var allTasks []Task
		for rows.Next() {
			var t Task
			rows.Scan(&t.ID, &t.Name, &t.Command, &t.Status)
			allTasks = append(allTasks, t)
		}
		json.NewEncoder(w).Encode(allTasks)
	}

	if r.Method == "POST" {
		var task Task
		json.NewDecoder(r.Body).Decode(&task)
		_, err := db.Exec(
			"INSERT INTO tasks (name, command) VALUES ($1, $2)",
			task.Name, task.Command,
		)
		if err != nil {
			fmt.Fprintln(w, "db error:", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status": "created"}`)
	}
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "server is running!")
}

func startScheduler() {
	c := cron.New()
	c.AddFunc("* * * * *", func() {
		rows, err := db.Query("SELECT id FROM tasks WHERE schedule = '* * * * *'")
		if err != nil {
			slog.Error("scheduler error", "error", err)
			return
		}
		for rows.Next() {
			var id int
			rows.Scan(&id)
			err := rdb.LPush(ctx, "job_queue", id).Err()
			if err != nil {
				slog.Error("scheduler queue error", "error", err)
			} else {
				slog.Info("scheduler queued task", "task_id", id)
			}
		}
	})
	c.Start()
	slog.Info("scheduler started")
}

func trackMetrics(path string, next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        next(w, r)
        requestCount.WithLabelValues(r.Method, path).Inc()
        requestDuration.WithLabelValues(path).Observe(time.Since(start).Seconds())
    }
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))
	prometheus.MustRegister(requestCount)
    prometheus.MustRegister(requestDuration)
	initDB()
	initRedis()

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/tasks", trackMetrics("/tasks", rateLimitMiddleware(authMiddleware(handleTasks))))
	http.HandleFunc("/execute/", trackMetrics("/tasks", rateLimitMiddleware(authMiddleware(handleExecute))))
	http.HandleFunc("/logs/", trackMetrics("/tasks", rateLimitMiddleware(authMiddleware(handleLogs))))
	http.HandleFunc("/register/", handleRegister)
	http.HandleFunc("/login/", handleLogin)
	http.Handle("/metrics", promhttp.Handler())

	slog.Info("server starting", "first port", 9090)
	startScheduler()
	
    port := os.Getenv("PORT")
    if port == "" {
        port = "9090"
    }
	err := http.ListenAndServe(":"+port, nil)
	slog.Error("server error", "error", err)
}
