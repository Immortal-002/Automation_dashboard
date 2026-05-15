package main

import (
	"fmt"
    "net/http"
	"encoding/json"
	"database/sql"
	_ "github.com/lib/pq"
    "github.com/redis/go-redis/v9"
	"context"
)

type Task struct {
	Name string `json:"name"`
	Command string `json:"command"`
	ID int `json:"id"`
	Status string `json:"status"`
}
type Log struct {
	ID int `json:"id"`
	TaskID int `json:"task_id"`
	Output string `json:"output"`
	Status string `json:"status"`
}

var db *sql.DB
var rdb *redis.Client
var ctx = context.Background()
var tasks []Task
var job_logs []Log

func initDB() {
	connStr := "user=postgres dbname=automation sslmode=disable"
	var err error
	db,err = sql.Open("postgres", connStr)
	if err!= nil {
		fmt.Println("db connection error:", err)
	}
	fmt.Println("db connected!")
}

func initRedis() {
	rdb = redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	fmt.Println("redis connected!")
}

func enableCORS(w http.ResponseWriter) {
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func handleLogs(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method != "GET" {
		fmt.Fprintln(w, "method not allowed")
		return
	}
	id := r.URL.Path[len("/logs/"):]
	rows, err := db.Query(" Select id, task_id, output, status FROM job_logs WHERE task_id = $1", id)
	if err!=nil {
		fmt.Fprintln(w, "db error:", err)
		return
	}
    var allLogs [] Log
	for rows.Next() {
		var l Log
		rows.Scan(&l.ID, &l.TaskID, &l.Output, &l.Status)
		allLogs = append(allLogs, l)
	}
	json.NewEncoder(w).Encode(allLogs)
}

func handleExecute(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method != "POST" {
		fmt.Fprintln(w, "method not allowed")
		return
	}
	id := r.URL.Path[len("/execute/"):]
	err := rdb.LPush(ctx,"job_queue", id).Err()
	if err!=nil {
		fmt.Fprintln(w, "redis error:", err)
		return
	}
	fmt.Fprintln(w, "job queued! task id:", id)
}



func handleTasks(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "GET" {
		w.Header().Set("Content-Type", "application/json")
		rows, err := db.Query("Select id, name, command, status FROM tasks")
		if err!=nil {
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

		
		fmt.Fprintln(w, "creating tasks...")
		var task Task
		json.NewDecoder(r.Body).Decode(&task)
		task.ID=len(tasks)+1
	    _, err := db.Exec(
			"INSERT INTO tasks (name, command) VALUES ($1, $2)",
			task.Name, task.Command,
		)
		if err!= nil {
			fmt.Fprintln(w, "db error:", err)
			return
		}
		fmt.Fprintln(w,"task saved to d!", task.Name)
		fmt.Fprintln(w, "created task:", task.Name, task.Command)
	}

}


func handleHome(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintln(w, "server is running! ")
}

func main() {
    initDB()
	initRedis()


	http.HandleFunc("/", handleHome)
	http.HandleFunc("/tasks", handleTasks)
	http.HandleFunc("/execute/", handleExecute)
	http.HandleFunc("/logs/", handleLogs)
	fmt.Println("server starting on port 9090 ..")
	err := http.ListenAndServe(":9090",nil)
	fmt.Println("server error:", err)
}
