import React from 'react';
import { useStore } from '../store';
import { Maximize2, Minimize2, ChevronRight } from 'lucide-react';

export default function TopBar() {
  const {
    projects,
    currentProjectId,
    currentChapterId,
    currentSceneId,
    chapters,
    editorFocusMode,
    toggleFocusMode,
  } = useStore();

  const project = projects.find((p) => p.id === currentProjectId);
  const chapter = chapters.find((c) => c.id === currentChapterId);
  const scene = chapter?.scenes.find((s) => s.id === currentSceneId);

  let totalWordCount = 0;
  if (currentProjectId) {
    const projectChapters = chapters.filter((c) => c.projectId === currentProjectId);
    projectChapters.forEach((c) => {
      c.scenes.forEach((s) => {
        totalWordCount += s.wordCount || 0;
      });
    });
  }

  if (!project) {
    return (
      <div className="h-12 bg-[#161b22] border-b border-[#21262d] flex items-center px-4 shrink-0">
        <span className="text-gray-500 text-sm">No project selected</span>
      </div>
    );
  }

  return (
    <div className="h-12 bg-[#161b22] border-b border-[#21262d] flex items-center justify-between px-4 shrink-0 select-none">
      {/* Left side: Breadcrumb */}
      <div className="flex items-center gap-2 text-sm text-gray-300">
        <span className="font-semibold text-gray-200">{project.title}</span>
        
        {chapter && (
          <>
            <ChevronRight size={14} className="text-[#30363d]" />
            <span className="text-gray-400">{chapter.title}</span>
          </>
        )}
        
        {scene && (
          <>
            <ChevronRight size={14} className="text-[#30363d]" />
            <span className="text-gray-400">{scene.title}</span>
          </>
        )}
      </div>

      {/* Center: Empty */}
      <div className="flex-1"></div>

      {/* Right side: Word count and Focus mode */}
      <div className="flex items-center gap-4 text-xs font-medium text-gray-400">
        {project.wordCountGoal && project.wordCountGoal > 0 ? (
          <div className="flex items-center gap-1.5 bg-[#0d1117] px-2.5 py-1 rounded border border-[#30363d]">
            <span className="text-gray-300">{totalWordCount}</span>
            <span className="text-gray-500">/</span>
            <span>{project.wordCountGoal} words</span>
          </div>
        ) : (
          <div className="flex items-center gap-1.5 bg-[#0d1117] px-2.5 py-1 rounded border border-[#30363d]">
            <span className="text-gray-300">{totalWordCount}</span> words
          </div>
        )}

        <button 
          onClick={toggleFocusMode}
          className="p-1.5 hover:bg-[#30363d] rounded text-gray-400 hover:text-gray-200 transition-colors flex items-center justify-center"
          title={editorFocusMode ? "Exit Focus Mode" : "Enter Focus Mode"}
        >
          {editorFocusMode ? <Minimize2 size={16} /> : <Maximize2 size={16} />}
        </button>
      </div>
    </div>
  );
}
