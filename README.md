# Automation Dashboard

A distributed job execution system built from scratch — create tasks, run them asynchronously, and view execution logs. Similar in concept to how GitHub Actions and CI/CD runners work internally.

Built with Go, Redis, PostgreSQL, and React.

---

## What It Does

- Create tasks (name + shell command)
- Trigger execution via API
- Jobs queue in Redis — server never blocks
- Separate worker process picks jobs and runs them
- Execution output saved to database
- Retrieve logs per task via API
- React frontend for UI (in progress)

---

## Architecture

```
Browser / curl
      │
      ▼
 Backend API (Go)          ← handles HTTP, stores tasks, queues jobs
      │
      ├──► PostgreSQL       ← stores tasks + execution logs
      │
      └──► Redis Queue      ← job queue (FIFO)
                │
                ▼
           Worker (Go)      ← pulls jobs, runs commands, saves output
```

---

## Tech Stack

| Part | Tech |
|---|---|
| Backend API | Go (`net/http`) |
| Job Queue | Redis |
| Database | PostgreSQL |
| Worker | Go (`os/exec`) |
| Frontend | React + TypeScript (in progress) |

---

## API Endpoints

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/tasks` | Create a new task |
| `GET` | `/tasks` | Fetch all tasks |
| `POST` | `/execute/:id` | Queue task for execution |
| `GET` | `/logs/:id` | Get execution logs for a task |

---

## Project Structure

```
Automation_dashboard/
├── backend/
│   ├── main.go        # API server, routes, DB + Redis connection
│   └── go.mod
├── worker/
│   ├── main.go        # Job queue consumer, command executor
│   └── go.mod
└── frontend/          # React + TypeScript (in progress)
```

---

## Getting Started

### Requirements

- Go 1.21+
- PostgreSQL
- Redis

### Database Setup

```sql
CREATE DATABASE automation;

\c automation

CREATE TABLE tasks (
    id SERIAL PRIMARY KEY,
    name VARCHAR(100),
    command VARCHAR(255),
    status VARCHAR(50) DEFAULT 'pending',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE job_logs (
    id SERIAL PRIMARY KEY,
    task_id INTEGER REFERENCES tasks(id),
    output TEXT,
    status VARCHAR(50),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

### Run Backend

```bash
cd backend
go run main.go
# Server starts on port 9090
```

### Run Worker

```bash
cd worker
go run main.go
# Worker starts listening for jobs on Redis
```

---

## Usage Example

**Create a task:**
```bash
curl -X POST http://localhost:9090/tasks \
  -H "Content-Type: application/json" \
  -d '{"name": "List Files", "command": "ls"}'
```

**Queue it for execution:**
```bash
curl -X POST http://localhost:9090/execute/1
```

**Check logs:**
```bash
curl http://localhost:9090/logs/1
```

---

## What I Learned Building This

- How async job queues work (Redis FIFO, BRPop blocking)
- Why backend APIs should never execute long-running tasks directly
- PostgreSQL schema design — foreign keys, parameterized queries, SQL injection prevention
- Go concurrency basics — goroutines, blocking calls, context
- Producer-consumer pattern used in real distributed systems
- How CI/CD runners like GitHub Actions work under the hood

---

## Roadmap

- [x] Backend API (Go)
- [x] PostgreSQL integration
- [x] Redis job queue
- [x] Worker execution engine
- [x] Execution logs storage
- [ ] React + TypeScript frontend
- [ ] Task scheduler (cron)
- [ ] JWT authentication
- [ ] Docker deployment

---

## Author

**Saawan Rana** — [github.com/Immortal-002](https://github.com/Immortal-002)# Automation_dashboard
# Automation_dashboard
