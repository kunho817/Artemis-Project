export interface Project {
  id: string;
  title: string;
  author: string;
  description: string;
  createdAt: number;
  updatedAt: number;
  wordCountGoal: number;
}

export interface Scene {
  id: string;
  chapterId: string;
  title: string;
  content: string;
  order: number;
  synopsis: string;
  status: "draft" | "revised" | "final";
  wordCount: number;
  notes: string;
}

export interface Chapter {
  id: string;
  projectId: string;
  title: string;
  order: number;
  scenes: Scene[];
}

export interface Character {
  id: string;
  projectId: string;
  name: string;
  role: "protagonist" | "antagonist" | "supporting" | "minor";
  description: string;
  notes: string;
}

export interface WorldNote {
  id: string;
  projectId: string;
  category: "location" | "item" | "lore" | "timeline" | "other";
  title: string;
  content: string;
}

export interface AppState {
  projects: Project[];
  currentProjectId: string | null;
  currentChapterId: string | null;
  currentSceneId: string | null;
  sidebarView: "chapters" | "characters" | "world" | "notes";
  editorFocusMode: boolean;
  wordCountToday: number;
}

export function generateId(): string {
  return crypto.randomUUID();
}
