export interface StudyMetrics {
  dailyCardCount: number
  retentionRate: number // 0.0 – 1.0
  currentStreak: number // days
  totalReviewed: number
}
