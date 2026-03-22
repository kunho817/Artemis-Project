import React from 'react';
import TopBar from './components/TopBar';
import Sidebar from './components/Sidebar';
import Editor from './components/Editor';
import { useStore } from './store';
import './index.css';

export default function App() {
  const projects = useStore((state) => state.projects);
  const currentProjectId = useStore((state) => state.currentProjectId);
  const createProject = useStore((state) => state.createProject);
  const editorFocusMode = useStore((state) => state.editorFocusMode);

  const handleCreateProject = () => {
    const title = window.prompt('Project Title:');
    if (!title) return;
    const author = window.prompt('Author Name:') || '';
    createProject(title, author);
  };

  if (projects.length === 0 || !currentProjectId) {
    return (
      <div
        style={{
          display: 'flex',
          flexDirection: 'column',
          height: '100vh',
          alignItems: 'center',
          justifyContent: 'center',
          backgroundColor: '#0d1117',
          color: '#c9d1d9',
        }}
      >
        <h1 style={{ marginBottom: '1.5rem', fontWeight: 600 }}>Welcome to Artemis</h1>
        <button
          onClick={handleCreateProject}
          style={{
            padding: '10px 20px',
            backgroundColor: '#238636',
            color: '#ffffff',
            border: 'none',
            borderRadius: '6px',
            fontSize: '1rem',
            cursor: 'pointer',
            fontWeight: 500,
          }}
        >
          Create New Project
        </button>
      </div>
    );
  }

  return (
    <div
      style={{
        display: 'flex',
        flexDirection: 'column',
        height: '100vh',
        width: '100vw',
        overflow: 'hidden',
      }}
    >
      <TopBar />
      <div style={{ display: 'flex', flexDirection: 'row', flex: 1, overflow: 'hidden' }}>
        {!editorFocusMode && <Sidebar />}
        <div style={{ flexGrow: 1, overflow: 'hidden', display: 'flex' }}>
          <Editor />
        </div>
      </div>
    </div>
  );
}
