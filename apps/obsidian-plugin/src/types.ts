export interface Flashcard {
  id: string;
  question: string;
  answer: string;
  tags?: string[];
}

export interface Domain {
  id: string;
  name: string;
  description: string;
  concepts: Concept[];
}

export interface Concept {
  id: string;
  name: string;
  summary: string;
  flashcards: Flashcard[];
}

export interface StudyMetrics {
  dailyCardCount: number;
  retentionRate: number; // 0.0 – 1.0
  currentStreak: number; // days
  totalReviewed: number;
}
