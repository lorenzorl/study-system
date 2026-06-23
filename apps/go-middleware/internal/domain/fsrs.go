package domain

import "time"

// FSRSAlgorithm defines the contract for calculating the next review state.
// Implementations include DummyFSRS (grade-to-days mapping) and future
// real FSRS-5 math for production use.
type FSRSAlgorithm interface {
	CalculateNextState(current CardState, grade int) CardState
}

// DummyFSRS is a simple FSRS algorithm that maps grades to review intervals.
// Grade mapping: 1 (Again) → +0 days, 2 (Hard) → +1 day,
// 3 (Good) → +3 days, 4 (Easy) → +4 days.
type DummyFSRS struct{}

// CalculateNextState implements FSRSAlgorithm with a grade-to-days mapping.
// It increments Stability by 0.1, preserves Difficulty, and advances
// NextReview by the interval corresponding to the given grade.
func (d DummyFSRS) CalculateNextState(currentState CardState, grade int) CardState {
	daysMap := map[int]int{1: 0, 2: 1, 3: 3, 4: 4}
	days := daysMap[grade]
	now := time.Now()
	return CardState{
		Stability:   currentState.Stability + 0.1,
		Difficulty:  currentState.Difficulty,
		NextReview:  now.AddDate(0, 0, days),
		LastReview:  now,
		FlashcardID: currentState.FlashcardID,
	}
}
