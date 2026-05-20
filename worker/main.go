package main

import (
    "strconv"
    "context"
    "database/sql"
    "fmt"
    "os/exec"
    "github.com/redis/go-redis/v9"
    _ "github.com/lib/pq"
    "time"
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
    var out []byte
    var execErr error    
    for i :=0; i<3; i++ {
        out , execErr = exec.Command("bash", "-c", command).Output()
        if execErr == nil {
             break
        }
        fmt.Printf("attempt %d failed: %v\n", i+1, execErr)
        time.Sleep(time.Duration(5*(i+1))*time.Second)
    } 

    id, _ := strconv.Atoi(taskID)
    if execErr != nil {
        fmt.Println("job failed after 3 attempts")
        db.Exec("INSERT INTO job_logs (task_id, output, status) VALUES ($1, $2, $3)",
            id, execErr.Error(), "failed")
        return   


    }
        fmt.Println("output:", string(out))
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
