
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
	"io"
	"path/filepath"
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
	DependsOn *int `json:"depends_on"`
}

type Log struct {
	ID     int    `json:"id"`
	TaskID int    `json:"task_id"`
	Output string `json:"output"`
	Status string `json:"status"`
}

var db *sql.DB
var rdb *redis.Client
 
type ctxKey string
const userIDKey ctxKey = "userID"
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

var jwtSecret = func() string {
    s := os.Getenv("JWT_SECRET")
    if s == "" {
        slog.Warn("JWT_SECRET not set, using default — DO NOT use in production")
        return "secret_key"
    }
    return s
}()
func initDB() error{
	host := os.Getenv("DB_HOST")
    if host == "" {
        host = "localhost"
    }
	connStr := fmt.Sprintf("host=%s user=postgres password=postgres dbname=automation sslmode=disable", host)
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
    // retry up to 10 times, waiting 2 seconds between attempts
    for i := 0; i < 10; i++ {
        ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
        err = db.PingContext(ctx)
        cancel()
        if err == nil {
            return nil
        }
        slog.Info("waiting for postgres...", "attempt", i+1)
        time.Sleep(2 * time.Second)
    }
    return fmt.Errorf("ping db after retries: %w", err)
}

func initRedis() error{
	host := os.Getenv("REDIS_HOST")
    if host == "" {
        host = "localhost"
    }
	rdb = redis.NewClient(&redis.Options{
		Addr: host + ":6379",
	})
	// retry up to 10 times, waiting 2 seconds between attempts
	var err error
    for i := 0; i < 10; i++ {
        ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := rdb.Ping(ctx).Err()
        cancel()
        if err == nil {
            return nil
		}
        slog.Info("waiting for redis...", "attempt", i+1)
        time.Sleep(2 * time.Second)
    }
    return fmt.Errorf("ping rdb after retries: %w", err)
    

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
            w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, "no token provided")
			return
		}
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return []byte(jwtSecret), nil
		})
		if err != nil || !token.Valid {
            w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintln(w, "invalid token")
			return
		}
		claims, ok := token.Claims.(jwt.MapClaims)
        userIDFloat, ok2 := claims["user_id"].(float64)
        if !ok || !ok2 {
            w.WriteHeader(http.StatusUnauthorized)
            fmt.Fprintln(w, "invalid token claims")
            return
        }
        ctx := context.WithValue(r.Context(), userIDKey, int(userIDFloat))
        next(w, r.WithContext(ctx))
		//next(w, r)
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

func handleDelete(w http.ResponseWriter, r *http.Request) {
    enableCORS(w)
    if r.Method != "POST" { return }
    
    id := r.URL.Path[len("/delete/"):]
    
    // delete logs first (foreign key constraint)
    db.Exec("DELETE FROM job_logs WHERE task_id = $1", id)
    
    // then delete task
    _, err := db.Exec("DELETE FROM tasks WHERE id = $1", id)
    if err != nil {
        fmt.Fprintln(w, "db error:", err)
        return
    }
    fmt.Fprint(w, `{"status": "deleted"}`)
}


func handleCancel(w http.ResponseWriter, r *http.Request) {
    enableCORS(w)
    if r.Method != "POST" { return }
    
    id := r.URL.Path[len("/cancel/"):]
    _, err := db.Exec("UPDATE tasks SET cancelled = true WHERE id = $1", id)
    if err != nil {
        fmt.Fprintln(w, "db error:", err)
        return
    }
    fmt.Fprint(w, `{"status": "cancelled"}`)
}

func handleCheckEmail(w http.ResponseWriter, r *http.Request) {
    enableCORS(w)
    if r.Method != "POST" {
        return
    }
    var body struct {
        Email string `json:"email"`
    }
    json.NewDecoder(r.Body).Decode(&body)

    var count int
    db.QueryRow("SELECT COUNT(*) FROM users WHERE email = $1", 
        body.Email).Scan(&count)

    w.Header().Set("Content-Type", "application/json")
    if count > 0 {
        fmt.Fprint(w, `{"exists": true}`)
    } else {
        fmt.Fprint(w, `{"exists": false}`)
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
		"exp":     time.Now().Add(48 * time.Hour).Unix(),
	})
	tokenString, _ := token.SignedString([]byte(jwtSecret))
	fmt.Fprint(w, tokenString)
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method != "GET" {
		fmt.Fprintln(w, "method not allowed")
		return
	}
	userID, ok := getUserIDFromContext(r)
    if !ok {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }
	id := r.URL.Path[len("/logs/"):]
	 if id == "" {
        http.Error(w, "task_id required", http.StatusBadRequest)
        return
    }
	rows, err := db.Query("SELECT id, task_id, output, status FROM job_logs WHERE task_id = $1", id)
	if err != nil {
		// CORRECT — returns 500 with generic message, logs details server-side
        slog.Error("db operation failed", "error", err,"task_id", id, "user_id", userID, "endpoint", "logs")
        http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
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
		 http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := getUserIDFromContext(r)
    if !ok {
        http.Error(w, "unauthorized", http.StatusUnauthorized)
        return
    }
	id := r.URL.Path[len("/execute/"):]
	if id == "" {
        http.Error(w, "task id required", http.StatusBadRequest)
        return
	}
    var ownerID int
    err := db.QueryRow("SELECT user_id FROM tasks WHERE id = $1", id).Scan(&ownerID)
	 if err == sql.ErrNoRows {
        http.Error(w, "task not found", http.StatusNotFound)
        return
    }
	if err != nil {
        slog.Error("db query failed", "error", err)
        http.Error(w, "internal server error", http.StatusInternalServerError)
        return
    }
    if ownerID != userID {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }

	ctx := r.Context()
	err = rdb.LPush(ctx, "job_queue", id).Err()
	if err != nil {
		// CORRECT — returns 500 with generic message, logs details server-side
        slog.Error("redis operation failed", "error", err, "endpoint", "execute")
        http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintln(w, "job queued! task id:", id)
}

// getUserIDFromContext extracts the user ID from request context safely
func getUserIDFromContext(r *http.Request) (int, bool) {
	userID, ok := r.Context().Value(userIDKey).(int)
	return userID, ok
}
func handleTasks(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)

// extract userID once — available to both GET and POST
//    tokenString := r.Header.Get("Authorization")
//    token, _ := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
//        return []byte(jwtSecret), nil
//    })
//    claims := token.Claims.(jwt.MapClaims)
//    userID := int(claims["user_id"].(float64))
    userID, ok := getUserIDFromContext(r)  
	if !ok {  
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, "unauthorized: user context missing")  
		return  
	}  
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")

// then filter by user
        rows, err := db.Query("SELECT id, name, command, status, depends_on FROM tasks WHERE user_id = $1", userID)
		if err != nil {
			fmt.Fprintln(w, "db error:", err)
			return
		}
		var allTasks []Task
		for rows.Next() {
			var t Task
			rows.Scan(&t.ID, &t.Name, &t.Command, &t.Status, &t.DependsOn)
			allTasks = append(allTasks, t)
		}
		json.NewEncoder(w).Encode(allTasks)
	}

	if r.Method == "POST" {
		var task Task
		json.NewDecoder(r.Body).Decode(&task)
        if task.Name == "" || task.Command == "" {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusBadRequest) // sends 400
            fmt.Fprint(w, `{"error": "name and command are required"}`)
            return
        }
		_, err := db.Exec(
			"INSERT INTO tasks (name, command, depends_on, user_id) VALUES ($1, $2, $3, $4)",
			task.Name, task.Command, task.DependsOn, userID,
		)
		if err != nil {
			fmt.Fprintln(w, "db error:", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status": "created"}`)
	}
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method != "POST" {
		fmt.Fprintln(w, "method not allowed")
		return 
	}

	r.ParseMultipartForm(10<<20)
	file, handler, err := r.FormFile("script")
	if err != nil {
		fmt.Fprintln(w, "error getting file:", err)
        return
    }
    defer file.Close()
	ext := filepath.Ext(handler.Filename)
    if ext != ".py" && ext != ".sh" {
        fmt.Fprintln(w, "only .py and .sh files allowed")
        return
    }
	dst, err := os.Create("/home/rana/automation-dashboard/scripts/" + handler.Filename)
    if err != nil {
        fmt.Fprintln(w, "error saving file:", err)
        return
    }
    defer dst.Close()
    io.Copy(dst, file)

    fmt.Fprintln(w, "script uploaded:", handler.Filename)
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "server is running!")
}



func handleMe(w http.ResponseWriter, r *http.Request) {
    enableCORS(w)
    if r.Method != "GET" {
        return
    }

    // Step 1: get user ID from token
    tokenString := r.Header.Get("Authorization")
    token, _ := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
        return []byte(jwtSecret), nil
    })
    claims := token.Claims.(jwt.MapClaims)
    userID := int(claims["user_id"].(float64))

    // Step 2: query DB using that ID
    var email, assistantName string
    err := db.QueryRow(
        "SELECT email, assistant_name FROM users WHERE id = $1",
        userID,
    ).Scan(&email, &assistantName)
    if err != nil {
        fmt.Fprintln(w, "user not found")
        return
    }

    // Step 3: return JSON
    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(map[string]string{
        "email":          email,
        "assistant_name": assistantName,
    })
}

func handleMeAssistant(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method != "POST" {
		return
	}
	tokenString := r.Header.Get("Authorization")
	token, _ := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(jwtSecret), nil
	})

	claims := token.Claims.(jwt.MapClaims)
	userID := int(claims["user_id"].(float64))

	var body struct {
		AssistantName string `json:"assistant_name"`
	}
	json.NewDecoder(r.Body).Decode(&body)

	_, err := db.Exec(
		"UPDATE users SET assistant_name = $1 WHERE id = $2",
		body.AssistantName, userID, )
		if err!= nil {
			fmt.Fprintln(w, "db error:", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status": "saved"}`)
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
			ctx := context.Background()
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
	

    if err := initDB(); err != nil {
	    slog.Error("db init failed", "error", err)
	    return
    }
    slog.Info("db connected")

    if err := initRedis(); err != nil {
	    slog.Error("redis init failed", "error", err)
	    return
    }
	slog.Info("redis connected")

	http.HandleFunc("/", handleHome)
	http.HandleFunc("/tasks", trackMetrics("/tasks", rateLimitMiddleware(authMiddleware(handleTasks))))
	http.HandleFunc("/execute/", trackMetrics("/execute", rateLimitMiddleware(authMiddleware(handleExecute))))
	http.HandleFunc("/logs/", trackMetrics("/logs", rateLimitMiddleware(authMiddleware(handleLogs))))
	http.HandleFunc("/register/", handleRegister)
	http.HandleFunc("/login/", handleLogin)
	http.HandleFunc("/upload", handleUpload)
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/me",authMiddleware(handleMe))
	http.HandleFunc("/me/assistant", authMiddleware(handleMeAssistant))
    http.HandleFunc("/check-email", handleCheckEmail)
    http.HandleFunc("/cancel/", authMiddleware(handleCancel))

    http.HandleFunc("/delete/", authMiddleware(handleDelete))
	slog.Info("server starting", "first port", 9090)
	startScheduler()
	
    port := os.Getenv("PORT")
    if port == "" {
        port = "9090"
    }
	err := http.ListenAndServe(":"+port, nil)
	slog.Error("server error", "error", err)
}
