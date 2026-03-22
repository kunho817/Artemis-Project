import React, { useState, useMemo } from "react";
import { ChevronRight, ChevronDown, Plus, FileText, User, Globe } from "lucide-react";
import { useStore } from "../store";

export default function Sidebar() {
  const {
    currentProjectId,
    sidebarView,
    setSidebarView,
    chapters,
    characters,
    worldNotes,
    currentSceneId,
    setCurrentScene,
    setCurrentChapter,
    addChapter,
    addScene,
    addCharacter,
    addWorldNote,
  } = useStore();

  const [expandedChapters, setExpandedChapters] = useState<Set<string>>(new Set());

  const projectChapters = useMemo(() => {
    if (!currentProjectId) return [];
    return chapters
      .filter((c) => c.projectId === currentProjectId)
      .sort((a, b) => a.order - b.order);
  }, [chapters, currentProjectId]);

  const projectCharacters = useMemo(() => {
    if (!currentProjectId) return [];
    return characters.filter((c) => c.projectId === currentProjectId);
  }, [characters, currentProjectId]);

  const projectWorldNotes = useMemo(() => {
    if (!currentProjectId) return [];
    return worldNotes.filter((w) => w.projectId === currentProjectId);
  }, [worldNotes, currentProjectId]);

  const worldNotesByCategory = useMemo(() => {
    const groups: Record<string, typeof projectWorldNotes> = {
      location: [],
      item: [],
      lore: [],
      timeline: [],
      other: [],
    };
    for (const n of projectWorldNotes) {
      if (groups[n.category]) {
        groups[n.category].push(n);
      } else {
        groups.other.push(n);
      }
    }
    return groups;
  }, [projectWorldNotes]);

  if (!currentProjectId) {
    return (
      <div style={{ width: 280, backgroundColor: "#0d1117", borderRight: "1px solid #21262d" }} />
    );
  }

  const toggleChapter = (chapterId: string) => {
    setExpandedChapters((prev) => {
      const next = new Set(prev);
      if (next.has(chapterId)) {
        next.delete(chapterId);
      } else {
        next.add(chapterId);
      }
      return next;
    });
  };

  const handleAddChapter = () => {
    const title = window.prompt("Chapter Title:");
    if (title) addChapter(title);
  };

  const handleAddScene = (chapterId: string, e: React.MouseEvent) => {
    e.stopPropagation();
    const title = window.prompt("Scene Title:");
    if (title) {
      addScene(chapterId, title);
      setExpandedChapters((prev) => new Set(prev).add(chapterId));
    }
  };

  const handleAddCharacter = () => {
    const name = window.prompt("Character Name:");
    if (name) addCharacter(name, "supporting");
  };

  const handleAddWorldNote = () => {
    const title = window.prompt("Note Title:");
    if (title) addWorldNote("other", title);
  };

  const getStatusColor = (status: string) => {
    if (status === "draft") return "#d29922";
    if (status === "revised") return "#58a6ff";
    if (status === "final") return "#3fb950";
    return "#8b949e";
  };

  const styles = `
    .sidebar-container {
      width: 280px;
      min-width: 280px;
      background-color: #0d1117;
      border-right: 1px solid #21262d;
      display: flex;
      flex-direction: column;
      height: 100%;
      color: #c9d1d9;
      user-select: none;
    }
    .sidebar-tabs {
      display: flex;
      border-bottom: 1px solid #21262d;
      padding: 8px;
      gap: 4px;
    }
    .sidebar-tab {
      padding: 4px;
      cursor: pointer;
      border-radius: 6px;
      background-color: transparent;
      color: #8b949e;
      font-size: 12px;
      flex: 1;
      text-align: center;
    }
    .sidebar-tab:hover {
      color: #c9d1d9;
    }
    .sidebar-tab.active {
      background-color: #21262d;
      color: #c9d1d9;
      font-weight: 600;
    }
    .sidebar-content {
      flex: 1;
      overflow-y: auto;
      padding: 16px 8px;
    }
    .sidebar-section-header {
      display: flex;
      justify-content: space-between;
      align-items: center;
      margin-bottom: 12px;
      padding: 0 8px;
    }
    .sidebar-section-title {
      font-size: 14px;
      font-weight: 600;
      color: #c9d1d9;
    }
    .sidebar-icon-btn {
      background: none;
      border: none;
      color: #8b949e;
      cursor: pointer;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 4px;
      border-radius: 4px;
    }
    .sidebar-icon-btn:hover {
      background-color: #21262d;
      color: #c9d1d9;
    }
    .sidebar-item {
      display: flex;
      align-items: center;
      padding: 6px 8px;
      cursor: pointer;
      border-radius: 6px;
      color: #c9d1d9;
      margin-bottom: 2px;
    }
    .sidebar-item:hover {
      background-color: #21262d;
    }
    .sidebar-item.active {
      background-color: #1f6feb;
      color: #ffffff;
    }
    .sidebar-item.active .sidebar-icon {
      color: #ffffff;
    }
    .sidebar-icon {
      color: #8b949e;
      margin-right: 8px;
      flex-shrink: 0;
    }
    .sidebar-status-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      margin-right: 8px;
      flex-shrink: 0;
    }
    .sidebar-item-text {
      font-size: 13px;
      flex: 1;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .sidebar-category-title {
      font-size: 11px;
      font-weight: 600;
      color: #8b949e;
      text-transform: uppercase;
      padding: 4px 8px;
      letter-spacing: 0.05em;
      margin-top: 8px;
    }
    .sidebar-empty-state {
      padding: 16px 8px;
      color: #8b949e;
      font-size: 13px;
      text-align: center;
    }
  `;

  return (
    <div className="sidebar-container">
      <style>{styles}</style>
      
      <div className="sidebar-tabs">
        {(["chapters", "characters", "world", "notes"] as const).map((view) => (
          <div
            key={view}
            className={`sidebar-tab ${sidebarView === view ? "active" : ""}`}
            onClick={() => setSidebarView(view)}
          >
            {view.charAt(0).toUpperCase() + view.slice(1)}
          </div>
        ))}
      </div>

      <div className="sidebar-content">
        {sidebarView === "chapters" && (
          <>
            <div className="sidebar-section-header">
              <span className="sidebar-section-title">Chapters</span>
              <button className="sidebar-icon-btn" onClick={handleAddChapter} title="Add Chapter">
                <Plus size={16} />
              </button>
            </div>
            {projectChapters.length === 0 && (
              <div className="sidebar-empty-state">No chapters yet. Add one to get started.</div>
            )}
            {projectChapters.map((chapter) => (
              <div key={chapter.id} style={{ marginBottom: 4 }}>
                <div className="sidebar-item" onClick={() => toggleChapter(chapter.id)}>
                  {expandedChapters.has(chapter.id) ? (
                    <ChevronDown size={16} className="sidebar-icon" style={{ marginRight: 4 }} />
                  ) : (
                    <ChevronRight size={16} className="sidebar-icon" style={{ marginRight: 4 }} />
                  )}
                  <span className="sidebar-item-text" style={{ fontSize: 14, fontWeight: 500 }}>
                    {chapter.title}
                  </span>
                  <button
                    className="sidebar-icon-btn"
                    onClick={(e) => handleAddScene(chapter.id, e)}
                    title="Add Scene"
                  >
                    <Plus size={14} />
                  </button>
                </div>
                
                {expandedChapters.has(chapter.id) && (
                  <div style={{ marginLeft: 16 }}>
                    {chapter.scenes
                      .slice()
                      .sort((a, b) => a.order - b.order)
                      .map((scene) => (
                        <div
                          key={scene.id}
                          className={`sidebar-item ${currentSceneId === scene.id ? "active" : ""}`}
                          onClick={() => {
                            setCurrentChapter(chapter.id);
                            setCurrentScene(scene.id);
                          }}
                        >
                          <div
                            className="sidebar-status-dot"
                            style={{ backgroundColor: getStatusColor(scene.status) }}
                          />
                          <FileText size={14} className="sidebar-icon" style={{ marginRight: 6 }} />
                          <span className="sidebar-item-text">{scene.title}</span>
                        </div>
                      ))}
                    {chapter.scenes.length === 0 && (
                      <div className="sidebar-item-text" style={{ padding: "4px 8px 4px 28px", color: "#8b949e", fontSize: 12 }}>
                        No scenes
                      </div>
                    )}
                  </div>
                )}
              </div>
            ))}
          </>
        )}

        {sidebarView === "characters" && (
          <>
            <div className="sidebar-section-header">
              <span className="sidebar-section-title">Characters</span>
              <button className="sidebar-icon-btn" onClick={handleAddCharacter} title="Add Character">
                <Plus size={16} />
              </button>
            </div>
            {projectCharacters.length === 0 && (
              <div className="sidebar-empty-state">No characters added yet.</div>
            )}
            {projectCharacters.map((char) => (
              <div key={char.id} className="sidebar-item">
                <User size={16} className="sidebar-icon" />
                <div style={{ display: "flex", flexDirection: "column", flex: 1, overflow: "hidden" }}>
                  <span className="sidebar-item-text">{char.name}</span>
                  <span style={{ fontSize: 11, color: "#8b949e", textTransform: "capitalize" }}>
                    {char.role}
                  </span>
                </div>
              </div>
            ))}
          </>
        )}

        {sidebarView === "world" && (
          <>
            <div className="sidebar-section-header">
              <span className="sidebar-section-title">World Elements</span>
              <button className="sidebar-icon-btn" onClick={handleAddWorldNote} title="Add World Note">
                <Plus size={16} />
              </button>
            </div>
            {projectWorldNotes.length === 0 && (
              <div className="sidebar-empty-state">No world notes added yet.</div>
            )}
            {Object.entries(worldNotesByCategory).map(([cat, notes]) => {
              if (notes.length === 0) return null;
              return (
                <div key={cat} style={{ marginBottom: 12 }}>
                  <div className="sidebar-category-title">{cat}</div>
                  {notes.map((note) => (
                    <div key={note.id} className="sidebar-item">
                      <Globe size={14} className="sidebar-icon" />
                      <span className="sidebar-item-text">{note.title}</span>
                    </div>
                  ))}
                </div>
              );
            })}
          </>
        )}

        {sidebarView === "notes" && (
          <>
            <div className="sidebar-section-header">
              <span className="sidebar-section-title">Project Notes</span>
            </div>
            <div className="sidebar-empty-state">Notes view coming soon...</div>
          </>
        )}
      </div>
    </div>
  );
}
