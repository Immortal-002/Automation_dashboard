import React, { useState, useEffect } from 'react';

function App() {
    const [tasks, setTasks] = useState([]);
    const [name, setName] = useState('');
    const [command, setCommand] = useState('');
    const [logs, setLogs] = useState([]);
    const [selectedTask, setSelectedTask] = useState<number | null>(null);

    const fetchTasks = () => {
        fetch('http://localhost:9090/tasks')
        .then(res =>res.json())
        .then(data => setTasks(data));
    };

    const fetchLogs = (id: number) => {
      fetch(`http://localhost:9090/logs/${id}`)
      .then(res => res.json())
      .then(data => setLogs(data || []));
    };

    useEffect(() => {
        fetchTasks();
    }, []);

    const createTask = () => {
        fetch('http://localhost:9090/tasks', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ name, command })
    }).then(() => {
      setName('');
      setCommand('');
      fetchTasks();
    });
  };

    const runTask = (id: number) => {
        setSelectedTask(id);
      fetch(`http://localhost:9090/execute/${id}`, { method: 'POST' })
          .then(() => {
          setTimeout(() => fetchLogs(id), 2000);
      });
    };

    return (
    <div>
      <h1>Automation Dashboard</h1>

      <h2>Create Task</h2>
      <input value={name} onChange={e => setName(e.target.value)} placeholder="Task name" />
      <input value={command} onChange={e => setCommand(e.target.value)} placeholder="Command" />
      <button onClick={createTask}>Create</button>

      <h2>Tasks</h2>
      {tasks.map((task: any) => (
        <div key={task.id}>
          <p>{task.name} — {task.command} — {task.status}</p>
          <button onClick={() => runTask(task.id)}>Run</button>
        </div>
      ))}
      <h2>Logs {selectedTask ? `- Task #${selectedTask}` : ''}</h2>
         {logs.map((log: any) => (
         <p key={log.id}>{log.output} — {log.status}</p>
      ))}

    </div>
  );
}

export default App;



