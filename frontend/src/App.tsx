

import React, { useState, useEffect } from 'react';
import './App.css';

const API = 'http://localhost:9090';

function App() {
  
  const [token, setToken] = useState(localStorage.getItem('token') || '');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [assistantName, setAssistantName] = useState('Orion');
  const [username, setUsername] = useState('');
  const [isFirstTime, setIsFirstTime] = useState(false);
  const [newAssistantName, setNewAssistantName] = useState('');
  const [tasks, setTasks] = useState<any[]>([]);
  const [logs, setLogs] = useState<any[]>([]);
  const [selectedTask, setSelectedTask] = useState<number | null>(null);
  const [name, setName] = useState('');
  const [command, setCommand] = useState('');
  const [greeting, setGreeting] = useState('');
  const [showGreeting, setShowGreeting] = useState(false);

const [step, setStep] = useState<'email' | 'password'>('email');
const [userExists, setUserExists] = useState(false);

const [error, setError] = useState('');

  
const checkEmail = () => {
  fetch(`${API}/check-email`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email })
  })
  .then(res => res.json())
  .then(data => {
    setUserExists(data.exists);
    setStep('password');
  });
};


const logout = () => {
    setToken('');
    localStorage.removeItem('token');
};

const login = () => {
  const endpoint = userExists ? '/login/' : '/register/';
  fetch(`${API}${endpoint}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email, password })
  })
  .then(res => res.text())
  .then(t => {
    const cleanToken = t.trim();
    if (cleanToken.startsWith('eyJ')) {
      setError('');
      setToken(cleanToken);
      localStorage.setItem('token', cleanToken);
      fetchUserInfo(cleanToken);
    } else if (!userExists) {
      setError('');
      fetch(`${API}/login/`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password })
      })
      .then(res => res.text())
      .then(t2 => {
        const tok = t2.trim();
        setToken(tok);
        localStorage.setItem('token', tok);
        fetchUserInfo(tok);
      });
    } else {
      setError(t.trim());
    }
  });
};




  const fetchUserInfo = (tok: string) => {
    fetch(`${API}/me`, {
      headers: { 'Authorization': tok }
    })
    .then(res => res.json())
    .then(data => {
      if (data.assistant_name && data.assistant_name.trim() !== '') {
        setAssistantName(data.assistant_name);
        setUsername(data.email.split('@')[0]);
        const msg = `${data.assistant_name} welcomes you back, ${data.email.split('@')[0]}`;
        setGreeting(msg);
        setShowGreeting(true);
        setTimeout(() => setShowGreeting(false), 4000);
        fetchTasks(tok);
      } else {
        setIsFirstTime(true);
      }
    })
    .catch(() => {
      setGreeting(`Orion welcomes you back`);
      setShowGreeting(true);
      setTimeout(() => setShowGreeting(false), 4000);
    });
  };

  const saveAssistantName = () => {
    const name = newAssistantName.trim() || 'Orion';
    fetch(`${API}/me/assistant`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': token },
      body: JSON.stringify({ assistant_name: name })
    }).then(() => {
      setAssistantName(name);
      setIsFirstTime(false);
      setGreeting(`${name} is online. Welcome, ${username}`);
      setShowGreeting(true);
      setTimeout(() => setShowGreeting(false), 4000);
    });
  };


const fetchTasks = (tok?: string) => {
    const t = tok || token;
    if (!t) return;
    fetch(`${API}/tasks`, { headers: { 'Authorization': t } })
    .then(res => res.text())
    .then(text => {
        try {
            setTasks(JSON.parse(text) || []);
        } catch {
            logout();
        }
    });
};

  

  const fetchLogs = (id: number) => {
    fetch(`${API}/logs/${id}`, { headers: { 'Authorization': token } })
    .then(res => { if (!res.ok) return []; return res.json(); })
    .then(data => setLogs(data || []));
  };




useEffect(() => { if (token) fetchTasks(token); }, [token]);

  const createTask = () => {
    fetch(`${API}/tasks`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': token },
      body: JSON.stringify({ name, command })
    }).then(() => { setName(''); setCommand(''); fetchTasks(); });
  };

  const runTask = (id: number) => {
    setSelectedTask(id);
    fetch(`${API}/execute/${id}`, { method: 'POST', headers: { 'Authorization': token } })
    .then(() => { setTimeout(() => fetchLogs(id), 2000); });
  };

  // LOGIN SCREEN
if (!token) {
  return (
    <div className="login-screen">
      <div className="login-bg-grid" />
      <div className="login-orb" />
      <div className="login-box">
        <div className="login-logo">
          <span className="login-logo__mark">⬡</span>
          <span className="login-logo__name">ORION</span>
        </div>
        <p className="login-tagline">
          {step === 'email' ? 'Your distributed command center' : 
           userExists ? 'Welcome back, operator' : 'New operator detected'}
        </p>
        <div className="login-form">
          <div className="input-group">
            <label>EMAIL</label>
            <input
              value={email}
              onChange={e => setEmail(e.target.value)}
              placeholder="operator@domain.com"
              type="email"
              onKeyDown={e => e.key === 'Enter' && step === 'email' && checkEmail()}
              disabled={step === 'password'}
            />
          </div>
          {step === 'password' && (
            <div className="input-group">
              <label>{userExists ? 'PASSWORD' : 'CREATE PASSWORD'}</label>
              <input
                value={password}
                onChange={e => setPassword(e.target.value)}
                placeholder="••••••••"
                type="password"
                onKeyDown={e => e.key === 'Enter' && login()}
                autoFocus
              />
            </div>
          )}
          {step === 'email' ? (
            <button className="btn-primary" onClick={checkEmail}>
              CONTINUE →
            </button>
          ) : (
            <button className="btn-primary" onClick={login}>
              {userExists ? 'ACCESS SYSTEM' : 'INITIALIZE OPERATOR'}
            </button>

          )}
          {error && <p style={{color:'var(--danger)', fontSize:'11px', letterSpacing:'1px', marginTop:'8px'}}>{error.toUpperCase()}</p>}

          {step === 'password' && (
            <button 
              style={{background:'transparent', border:'none', color:'var(--text-muted)', cursor:'pointer', fontSize:'11px', letterSpacing:'2px'}}
              onClick={() => { setStep('email'); setPassword(''); }}
            >
              ← BACK
            </button>
          )}
        </div>
      </div>
      <div className="login-footer">AUTOMATION DASHBOARD v1.1</div>
    </div>
  );
}

  // FIRST TIME — NAME YOUR ASSISTANT
  if (isFirstTime) {
    return (
      <div className="login-screen">
        <div className="login-bg-grid" />
        <div className="login-box">
          <div className="login-logo">
            <span className="login-logo__mark">⬡</span>
            <span className="login-logo__name">WELCOME</span>
          </div>
          <p className="login-tagline">Name your assistant. It will serve you from here on.</p>
          <div className="input-group">
            <label>ASSISTANT NAME</label>
            <input
              value={newAssistantName}
              onChange={e => setNewAssistantName(e.target.value)}
              placeholder="e.g. Orion, Atlas, Nova..."
              onKeyDown={e => e.key === 'Enter' && saveAssistantName()}
            />
          </div>
          <button className="btn-primary" onClick={saveAssistantName}>
            ACTIVATE ASSISTANT
          </button>
          {error && <p style={{color:'var(--danger)', fontSize:'11px', letterSpacing:'1px', marginTop:'8px'}}>{error.toUpperCase()}</p>}
        </div>
      </div>
    );
  }

  // MAIN DASHBOARD
  return (
    <div className="dashboard">
      {/* GREETING OVERLAY */}
      {showGreeting && (
        <div className="greeting-overlay">
          <span className="greeting-text">{greeting}</span>
        </div>
      )}

      {/* SIDEBAR */}
       
<aside className="sidebar">
    <div className="sidebar-logo" onClick={logout} style={{cursor:'pointer'}} title="Settings">
        <span className="sidebar-logo__mark">⬡</span>
        <span className="sidebar-logo__text">ORION</span>
    </div>
    <nav className="sidebar-nav">
        <a className="sidebar-nav__item active" href="#">Tasks</a>
        <a className="sidebar-nav__item" href="#">Logs</a>
        <a className="sidebar-nav__item" href="#">Metrics</a>
        <a className="sidebar-nav__item" href="#">Scripts</a>
    </nav>
    <div className="sidebar-status">
        <span className="status-dot status-dot--online" />
        <span>System Online</span>
    </div>
    <button onClick={logout} style={{
        margin:'0 16px 16px',
        background:'transparent',
        border:'1px solid var(--bg-border)',
        color:'var(--text-muted)',
        fontFamily:'var(--font-display)',
        fontSize:'9px',
        letterSpacing:'2px',
        padding:'10px',
        cursor:'pointer'
    }}>LOGOUT</button>
</aside>

      {/* MAIN CONTENT */}
      <main className="main">
        {/* HEADER */}
        <header className="topbar">
          <div className="topbar-title">TASK CONTROL</div>
          <div className="topbar-right">
            <span className="topbar-user">{username}</span>
          </div>
        </header>

        {/* CREATE TASK */}
        <section className="panel">
          <h2 className="panel-title">NEW TASK</h2>
          <div className="create-form">
            <input
              className="field"
              value={name}
              onChange={e => setName(e.target.value)}
              placeholder="Task name"
            />
            <input
              className="field"
              value={command}
              onChange={e => setCommand(e.target.value)}
              placeholder="Command (e.g. echo hello)"
            />
            <button className="btn-primary" onClick={createTask}>DEPLOY</button>
          </div>
        </section>

        {/* TASK LIST */}
        <section className="panel">
          <h2 className="panel-title">ACTIVE TASKS <span className="panel-count">{tasks.length}</span></h2>
          <div className="task-list">
            {tasks.map((task: any) => (
              <div key={task.id} className="task-card">
                <div className="task-card__info">
                  <span className="task-card__id">#{task.id}</span>
                  <span className="task-card__name">{task.name}</span>
                  <code className="task-card__cmd">{task.command}</code>
                </div>
                <div className="task-card__right">
                  <span className={`badge badge--${task.status}`}>{task.status}</span>
                  <button className="btn-run" onClick={() => runTask(task.id)}>RUN ▶</button>
                </div>
              </div>
            ))}
            {tasks.length === 0 && <p className="empty-state">No tasks deployed yet.</p>}
          </div>
        </section>

        {/* LOGS */}
        {selectedTask && (
          <section className="panel">
            <h2 className="panel-title">EXECUTION LOGS — TASK #{selectedTask}</h2>
            <div className="log-viewer">
              {logs.map((log: any) => (
                <div key={log.id} className={`log-line log-line--${log.status}`}>
                  <span className="log-line__status">[{log.status.toUpperCase()}]</span>
                  <span className="log-line__output">{log.output}</span>
                </div>
              ))}
              {logs.length === 0 && <p className="log-empty">Awaiting execution output...</p>}
            </div>
          </section>
        )}
      </main>

      {/* ASSISTANT WIDGET */}
      <div className="assistant-widget">
        <div className="assistant-bubble">{assistantName} online</div>
        <div className="assistant-avatar">⬡</div>
      </div>
    </div>
  );
}

export default App;
