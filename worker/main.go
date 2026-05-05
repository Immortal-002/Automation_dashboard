package main

import (
    "strconv"
    "context"
    "database/sql"
    "fmt"
    "os/exec"
    "github.com/redis/go-redis/v9"
    _ "github.com/lib/pq"
)

var rdb *redis.Client
var db *sql.DB
var ctx = context.Background()

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


func processJob(taskID string) {
    var command string
	err := db.QueryRow("SELECT command FROM tasks WHERE id = $1", taskID).Scan(&command)
    if err != nil {
        fmt.Println("db error:", err)
        return
    }
    fmt.Println("running command:", command)
    out , err:= exec.Command("bash", "-c", command).Output()
    if err != nil {
        fmt.Println("exec error:", err)
        return 
    }
    fmt.Println("output:", string(out))
    id, _ := strconv.Atoi(taskID)
    db.Exec("INSERT INTO job_logs ( task_id, output, status) VALUES ($1, $2, $3)", id, string(out), "success")
}

func main() {
    initDB()
    initRedis()


    fmt.Println("worker started, waiting for jobs...")

    for {
        result, err := rdb.BRPop(ctx, 0, "job_queue").Result()
        if err != nil {
            fmt.Println("redis error:", err)
            continue
        }
        taskID := result[1]
        processJob(taskID)
    }
}
