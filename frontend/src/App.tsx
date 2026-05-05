import React, { useState, useEffect } from 'react';

function App() {
  const [tasks, setTasks] = useState([]);

  useEffect(() => {
    fetch('http://localhost:9090/tasks')
      .then(res => res.json())
      .then(data => setTasks(data));
  }, []);

  return (
    <div>
      <h1>Automation Dashboard</h1>
      <h2>Tasks</h2>
      {tasks.map((task: any) => (
        <div key={task.id}>
          <p>{task.name} — {task.command} — {task.status}</p>
        </div>
      ))}
    </div>
  );
}

export default App;
