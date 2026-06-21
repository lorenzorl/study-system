import type { Flashcard } from "./types";

export const SAMPLE_FLASHCARDS: Flashcard[] = [
  {
    id: "card-1",
    question: "What is the time complexity of binary search?",
    answer: "O(log n) — each step halves the search space.",
  },
  {
    id: "card-2",
    question: "What is a closure in JavaScript?",
    answer:
      "A closure is a function that retains access to its lexical scope even when executed outside of it.",
  },
  {
    id: "card-3",
    question: "What does SOLID stand for?",
    answer:
      "Single Responsibility, Open/Closed, Liskov Substitution, Interface Segregation, Dependency Inversion.",
  },
  {
    id: "card-4",
    question: "What is the difference between == and === in JavaScript?",
    answer:
      "=== is strict equality (no type coercion), == allows type coercion before comparison.",
  },
  {
    id: "card-5",
    question: "What is a Promise in JavaScript?",
    answer:
      "An object representing the eventual completion or failure of an asynchronous operation.",
  },
];
