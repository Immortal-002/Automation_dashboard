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
    "log/slog"
    "os"
	"strings"
	
)


var rdb *redis.Client
var db *sql.DB
var ctx = context.Background()

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


func processJob(taskID string) {
    var command string
	var dependsOn *int
	err := db.QueryRow("SELECT command, depends_on FROM tasks WHERE id = $1", taskID).Scan(&command, &dependsOn)
    if err != nil {
        fmt.Println("db error:", err)
        return
    }
var cancelled bool
db.QueryRow("SELECT cancelled FROM tasks WHERE id = $1", taskID).Scan(&cancelled)
if cancelled {
    slog.Info("task cancelled, skipping", "task_id", taskID)
    return
}
    if dependsOn != nil {
        if !isDependencyComplete(*dependsOn) {
            slog.Info("dependency not complete, requeueing", "task_id", taskID)
            time.Sleep(5 * time.Second)
            rdb.LPush(ctx, "job_queue", taskID)
            return
        }
    }
    slog.Info("running command:", command)
    var out []byte
    var execErr error  
   
    
if strings.HasPrefix(command, "script:") {
    scriptName := strings.TrimPrefix(command, "script:")
    scriptPath := "/home/rana/automation-dashboard/scripts/" + scriptName
    
    var dockerCmd string
    if strings.HasSuffix(scriptName, ".py") {
        dockerCmd = fmt.Sprintf(
            "docker run --rm --network none --memory 128m --cpus 0.5 -v %s:/script.py:ro python:3.11-alpine python /script.py",
            scriptPath,
        )
    } else if strings.HasSuffix(scriptName, ".sh") {
        dockerCmd = fmt.Sprintf(
            "docker run --rm --network none --memory 128m --cpus 0.5 -v %s:/script.sh:ro alpine sh /script.sh",
            scriptPath,
        )
    }
    command = dockerCmd
}



//	if strings.HasPrefix(command, "script:") {
  //  scriptName := strings.TrimPrefix(command, "script:")
 //   scriptPath := "/home/rana/automation-dashboard/scripts/" + scriptName
    
    // detect language
  //  if strings.HasSuffix(scriptName, ".py") {
      //  command = "python3 " + scriptPath
    //} else if strings.HasSuffix(scriptName, ".sh") {
  //      command = "bash " + scriptPath
   // }
//}

        
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()


    for i :=0; i<3; i++ {
        out , execErr = exec.CommandContext(ctx, "bash", "-c", command).Output()
        if execErr == nil {
             break
        }
        fmt.Printf("attempt %d failed: %v\n", i+1, execErr)
        time.Sleep(time.Duration(5*(i+1))*time.Second)
    } 

    id, _ := strconv.Atoi(taskID)
    if execErr != nil {
        slog.Error("job failed after 3 attempts")
        db.Exec("INSERT INTO job_logs (task_id, output, status) VALUES ($1, $2, $3)",
            id, execErr.Error(), "failed")
        return   


    }
        fmt.Println("output:", string(out))
        db.Exec("INSERT INTO job_logs ( task_id, output, status) VALUES ($1, $2, $3)", id, string(out), "success")
           

}

func isDependencyComplete(dependsOn int) bool {
    var status string
    err := db.QueryRow(`
        SELECT status FROM job_logs 
        WHERE task_id = $1 
        ORDER BY created_at DESC 
        LIMIT 1
    `, dependsOn).Scan(&status)
    if err != nil {
        return false
    }
    return status == "success"
}

func main() {
    slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})))
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



    slog.Info("worker started, waiting for jobs...")

    for {
        result, err := rdb.BRPop(ctx, 0, "job_queue").Result()
        if err != nil {
            slog.Error("redis error:", err)
            continue
        }
        taskID := result[1]
        processJob(taskID)
    }
}
