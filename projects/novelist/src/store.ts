import { create } from "zustand";
import { persist } from "zustand/middleware";
import {
  Project,
  Scene,
  Chapter,
  Character,
  WorldNote,
  AppState,
  generateId,
} from "./types";

interface StoreState extends AppState {
  chapters: Chapter[];
  characters: Character[];
  worldNotes: WorldNote[];

  createProject: (title: string, author: string) => void;
  deleteProject: (id: string) => void;
  setCurrentProject: (id: string | null) => void;

  addChapter: (title: string) => void;
  deleteChapter: (id: string) => void;
  setCurrentChapter: (id: string | null) => void;

  addScene: (chapterId: string, title: string) => void;
  deleteScene: (chapterId: string, sceneId: string) => void;
  setCurrentScene: (id: string | null) => void;
  updateSceneContent: (sceneId: string, content: string) => void;
  updateSceneField: <K extends keyof Scene>(sceneId: string, field: K, value: Scene[K]) => void;

  addCharacter: (name: string, role: Character["role"]) => void;
  deleteCharacter: (id: string) => void;
  updateCharacter: (id: string, updates: Partial<Character>) => void;

  addWorldNote: (category: WorldNote["category"], title: string) => void;
  deleteWorldNote: (id: string) => void;
  updateWorldNote: (id: string, updates: Partial<WorldNote>) => void;

  setSidebarView: (view: AppState["sidebarView"]) => void;
  toggleFocusMode: () => void;

  reorderChapters: (chapterIds: string[]) => void;
  reorderScenes: (chapterId: string, sceneIds: string[]) => void;
}

function touchProject(projects: Project[], projectId: string): Project[] {
  return projects.map((p) =>
    p.id === projectId ? { ...p, updatedAt: Date.now() } : p
  );
}

function calculateWordCount(text: string): number {
  return text.trim() ? text.trim().split(/\s+/).length : 0;
}

export const useStore = create<StoreState>()(
  persist(
    (set) => ({
      projects: [],
      currentProjectId: null,
      currentChapterId: null,
      currentSceneId: null,
      sidebarView: "chapters",
      editorFocusMode: false,
      wordCountToday: 0,
      chapters: [],
      characters: [],
      worldNotes: [],

      createProject: (title, author) =>
        set((state) => {
          const newProject: Project = {
            id: generateId(),
            title,
            author,
            description: "",
            createdAt: Date.now(),
            updatedAt: Date.now(),
            wordCountGoal: 0,
          };
          return {
            projects: [...state.projects, newProject],
            currentProjectId: newProject.id,
            currentChapterId: null,
            currentSceneId: null,
          };
        }),

      deleteProject: (id) =>
        set((state) => ({
          projects: state.projects.filter((p) => p.id !== id),
          currentProjectId: state.currentProjectId === id ? null : state.currentProjectId,
          chapters: state.chapters.filter((c) => c.projectId !== id),
          characters: state.characters.filter((c) => c.projectId !== id),
          worldNotes: state.worldNotes.filter((w) => w.projectId !== id),
          currentChapterId: state.currentProjectId === id ? null : state.currentChapterId,
          currentSceneId: state.currentProjectId === id ? null : state.currentSceneId,
        })),

      setCurrentProject: (id) =>
        set({
          currentProjectId: id,
          currentChapterId: null,
          currentSceneId: null,
        }),

      addChapter: (title) =>
        set((state) => {
          if (!state.currentProjectId) return state;
          const projectChapters = state.chapters.filter(
            (c) => c.projectId === state.currentProjectId
          );
          const newChapter: Chapter = {
            id: generateId(),
            projectId: state.currentProjectId,
            title,
            order: projectChapters.length,
            scenes: [],
          };
          return {
            chapters: [...state.chapters, newChapter],
            projects: touchProject(state.projects, state.currentProjectId),
          };
        }),

      deleteChapter: (id) =>
        set((state) => {
          const chapter = state.chapters.find((c) => c.id === id);
          if (!chapter) return state;
          return {
            chapters: state.chapters.filter((c) => c.id !== id),
            currentChapterId: state.currentChapterId === id ? null : state.currentChapterId,
            projects: touchProject(state.projects, chapter.projectId),
          };
        }),

      setCurrentChapter: (id) => set({ currentChapterId: id }),

      addScene: (chapterId, title) =>
        set((state) => {
          const chapterIndex = state.chapters.findIndex((c) => c.id === chapterId);
          if (chapterIndex === -1) return state;
          const chapter = state.chapters[chapterIndex];
          const newScene: Scene = {
            id: generateId(),
            chapterId,
            title,
            content: "",
            order: chapter.scenes.length,
            synopsis: "",
            status: "draft",
            wordCount: 0,
            notes: "",
          };
          const newChapters = [...state.chapters];
          newChapters[chapterIndex] = {
            ...chapter,
            scenes: [...chapter.scenes, newScene],
          };
          return {
            chapters: newChapters,
            projects: touchProject(state.projects, chapter.projectId),
          };
        }),

      deleteScene: (chapterId, sceneId) =>
        set((state) => {
          const chapterIndex = state.chapters.findIndex((c) => c.id === chapterId);
          if (chapterIndex === -1) return state;
          const chapter = state.chapters[chapterIndex];
          const newChapters = [...state.chapters];
          newChapters[chapterIndex] = {
            ...chapter,
            scenes: chapter.scenes.filter((s) => s.id !== sceneId),
          };
          return {
            chapters: newChapters,
            currentSceneId: state.currentSceneId === sceneId ? null : state.currentSceneId,
            projects: touchProject(state.projects, chapter.projectId),
          };
        }),

      setCurrentScene: (id) => set({ currentSceneId: id }),

      updateSceneContent: (sceneId, content) =>
        set((state) => {
          let projectId: string | null = null;
          const wordCount = calculateWordCount(content);

          const chapters = state.chapters.map((c) => {
            if (c.scenes.some((s) => s.id === sceneId)) {
              projectId = c.projectId;
              return {
                ...c,
                scenes: c.scenes.map((s) =>
                  s.id === sceneId ? { ...s, content, wordCount } : s
                ),
              };
            }
            return c;
          });

          if (!projectId) return state;

          return {
            chapters,
            projects: touchProject(state.projects, projectId),
          };
        }),

      updateSceneField: (sceneId, field, value) =>
        set((state) => {
          let projectId: string | null = null;
          const chapters = state.chapters.map((c) => {
            if (c.scenes.some((s) => s.id === sceneId)) {
              projectId = c.projectId;
              return {
                ...c,
                scenes: c.scenes.map((s) => {
                  if (s.id === sceneId) {
                    const updated = { ...s, [field]: value };
                    if (field === "content") {
                      updated.wordCount = calculateWordCount(value as string);
                    }
                    return updated;
                  }
                  return s;
                }),
              };
            }
            return c;
          });

          if (!projectId) return state;

          return {
            chapters,
            projects: touchProject(state.projects, projectId),
          };
        }),

      addCharacter: (name, role) =>
        set((state) => {
          if (!state.currentProjectId) return state;
          const newCharacter: Character = {
            id: generateId(),
            projectId: state.currentProjectId,
            name,
            role,
            description: "",
            notes: "",
          };
          return {
            characters: [...state.characters, newCharacter],
            projects: touchProject(state.projects, state.currentProjectId),
          };
        }),

      deleteCharacter: (id) =>
        set((state) => {
          const character = state.characters.find((c) => c.id === id);
          if (!character) return state;
          return {
            characters: state.characters.filter((c) => c.id !== id),
            projects: touchProject(state.projects, character.projectId),
          };
        }),

      updateCharacter: (id, updates) =>
        set((state) => {
          const character = state.characters.find((c) => c.id === id);
          if (!character) return state;
          return {
            characters: state.characters.map((c) =>
              c.id === id ? { ...c, ...updates } : c
            ),
            projects: touchProject(state.projects, character.projectId),
          };
        }),

      addWorldNote: (category, title) =>
        set((state) => {
          if (!state.currentProjectId) return state;
          const newNote: WorldNote = {
            id: generateId(),
            projectId: state.currentProjectId,
            category,
            title,
            content: "",
          };
          return {
            worldNotes: [...state.worldNotes, newNote],
            projects: touchProject(state.projects, state.currentProjectId),
          };
        }),

      deleteWorldNote: (id) =>
        set((state) => {
          const note = state.worldNotes.find((w) => w.id === id);
          if (!note) return state;
          return {
            worldNotes: state.worldNotes.filter((w) => w.id !== id),
            projects: touchProject(state.projects, note.projectId),
          };
        }),

      updateWorldNote: (id, updates) =>
        set((state) => {
          const note = state.worldNotes.find((w) => w.id === id);
          if (!note) return state;
          return {
            worldNotes: state.worldNotes.map((w) =>
              w.id === id ? { ...w, ...updates } : w
            ),
            projects: touchProject(state.projects, note.projectId),
          };
        }),

      setSidebarView: (view) => set({ sidebarView: view }),

      toggleFocusMode: () =>
        set((state) => ({ editorFocusMode: !state.editorFocusMode })),

      reorderChapters: (chapterIds) =>
        set((state) => {
          if (!state.currentProjectId) return state;
          const newChapters = state.chapters.map((c) => {
            const newOrder = chapterIds.indexOf(c.id);
            if (newOrder !== -1) {
              return { ...c, order: newOrder };
            }
            return c;
          });

          newChapters.sort((a, b) => {
            if (a.projectId === b.projectId) {
              return a.order - b.order;
            }
            return 0;
          });

          return {
            chapters: newChapters,
            projects: touchProject(state.projects, state.currentProjectId),
          };
        }),

      reorderScenes: (chapterId, sceneIds) =>
        set((state) => {
          const chapterIndex = state.chapters.findIndex((c) => c.id === chapterId);
          if (chapterIndex === -1) return state;

          const chapter = state.chapters[chapterIndex];
          const newScenes = chapter.scenes.map((s) => {
            const newOrder = sceneIds.indexOf(s.id);
            if (newOrder !== -1) {
              return { ...s, order: newOrder };
            }
            return s;
          });

          newScenes.sort((a, b) => a.order - b.order);

          const newChapters = [...state.chapters];
          newChapters[chapterIndex] = { ...chapter, scenes: newScenes };

          return {
            chapters: newChapters,
            projects: touchProject(state.projects, chapter.projectId),
          };
        }),
    }),
    {
      name: "artemis-storage",
    }
  )
);
