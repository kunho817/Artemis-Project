import React, { useMemo } from 'react';
import { useStore } from '../store';

export default function Editor() {
  const chapters = useStore((state) => state.chapters);
  const currentSceneId = useStore((state) => state.currentSceneId);
  const updateSceneContent = useStore((state) => state.updateSceneContent);
  const editorFocusMode = useStore((state) => state.editorFocusMode);

  const scene = useMemo(() => {
    if (!currentSceneId) return null;
    for (const chapter of chapters) {
      const found = chapter.scenes.find((s) => s.id === currentSceneId);
      if (found) return found;
    }
    return null;
  }, [chapters, currentSceneId]);

  if (!scene) {
    return (
      <div
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          height: '100%',
          width: '100%',
          backgroundColor: '#161b22',
          color: '#8b949e',
          fontFamily: 'sans-serif',
          fontSize: '1rem',
        }}
      >
        Select a scene to start writing
      </div>
    );
  }

  const handleChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    updateSceneContent(scene.id, e.target.value);
  };

  return (
    <div
      style={{
        position: 'relative',
        width: '100%',
        height: '100%',
        backgroundColor: '#161b22',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <textarea
        value={scene.content}
        onChange={handleChange}
        placeholder="Start writing..."
        style={{
          flex: 1,
          width: '100%',
          maxWidth: editorFocusMode ? '700px' : 'none',
          margin: editorFocusMode ? '0 auto' : '0',
          backgroundColor: 'transparent',
          color: '#c9d1d9',
          fontFamily: 'monospace',
          fontSize: editorFocusMode ? '1.25rem' : '1rem',
          lineHeight: 1.8,
          border: 'none',
          outline: 'none',
          padding: '2rem',
          resize: 'none',
          boxSizing: 'border-box',
        }}
      />
      <div
        style={{
          position: 'absolute',
          bottom: '1rem',
          right: '1.5rem',
          color: '#8b949e',
          fontFamily: 'sans-serif',
          fontSize: '0.875rem',
          pointerEvents: 'none',
        }}
      >
        {scene.wordCount} words
      </div>
    </div>
  );
}
