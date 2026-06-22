import type { Domain, StudyMetrics } from "./types";

export const MOCK_DOMAINS: Domain[] = [
  {
    id: "ddd",
    name: "Domain-Driven Design",
    description:
      "Strategic design patterns for modeling complex software systems around business domains.",
    concepts: [
      {
        id: "ubiquitous-language",
        name: "Ubiquitous Language",
        summary:
          "A common, rigorous language shared by domain experts and developers, used everywhere — conversations, code, diagrams, and documentation.",
        flashcards: [
          {
            id: "ddd-ul-1",
            question: "What is Ubiquitous Language?",
            answer:
              "A shared language between domain experts and developers that is used consistently in conversation, code, and documentation to eliminate translation errors.",
          },
          {
            id: "ddd-ul-2",
            question: "Why is Ubiquitous Language important?",
            answer:
              "It ensures the mental model of domain experts is reflected in the code. Without it, developers create their own abstractions that drift from the business reality.",
          },
          {
            id: "ddd-ul-3",
            question: "Where should Ubiquitous Language appear?",
            answer:
              "Everywhere — code (class/method names), conversations, user stories, documentation, and diagrams. Any discrepancy between any of these is technical debt.",
          },
        ],
      },
      {
        id: "bounded-context",
        name: "Bounded Context",
        summary:
          "A boundary within which a particular domain model applies. Different bounded contexts may have different models for the same business concept.",
        flashcards: [
          {
            id: "ddd-bc-1",
            question: "What is a Bounded Context?",
            answer:
              "An explicit boundary within which a domain model is defined and applicable. Inside the boundary, all terms have specific, unambiguous meaning.",
          },
          {
            id: "ddd-bc-2",
            question: "How do Bounded Contexts relate to microservices?",
            answer:
              "A well-designed microservice typically aligns with a single Bounded Context, owning its data and domain logic. The context boundary becomes the service boundary.",
          },
          {
            id: "ddd-bc-3",
            question:
              'What does "Customer" mean differently across contexts?',
            answer:
              "In a Sales context, Customer means buyer with purchase history. In a Support context, Customer means someone with open tickets. Same word, different model.",
          },
        ],
      },
      {
        id: "aggregate",
        name: "Aggregate",
        summary:
          "A cluster of domain objects treated as a single unit, with one root entity enforcing invariants for the entire cluster.",
        flashcards: [
          {
            id: "ddd-agg-1",
            question: "What is an Aggregate Root?",
            answer:
              "The single entry point to an aggregate that enforces all business invariants. External objects only hold references to the root, never to internal entities.",
          },
          {
            id: "ddd-agg-2",
            question: "What is the transaction rule for aggregates?",
            answer:
              "A transaction should only modify one aggregate. Changes to multiple aggregates should be handled through domain events in eventual consistency patterns.",
          },
        ],
      },
    ],
  },
  {
    id: "architecture",
    name: "Software Architecture",
    description:
      "High-level structures, patterns, and principles that guide system design.",
    concepts: [
      {
        id: "clean-architecture",
        name: "Clean Architecture",
        summary:
          "A layered architecture where dependencies point inward. The domain core has zero external dependencies.",
        flashcards: [
          {
            id: "arch-ca-1",
            question: "What is the Dependency Rule in Clean Architecture?",
            answer:
              "Source code dependencies can only point inward. Outer layers depend on inner layers, never the reverse. The domain core knows nothing about frameworks or databases.",
          },
          {
            id: "arch-ca-2",
            question: "What are the four concentric layers?",
            answer:
              "From inner to outer: Entities (domain objects), Use Cases (application logic), Interface Adapters (controllers, presenters, gateways), and Frameworks & Drivers (web, DB, UI).",
          },
          {
            id: "arch-ca-3",
            question: "Why isolate the domain core?",
            answer:
              "So business rules can be tested independently, swapped between frameworks, and protected from infrastructure changes. The domain should outlive any particular framework.",
          },
        ],
      },
      {
        id: "cqrs",
        name: "CQRS",
        summary:
          "Command Query Responsibility Segregation — separate models for reads (queries) and writes (commands).",
        flashcards: [
          {
            id: "arch-cqrs-1",
            question: "What is the core idea of CQRS?",
            answer:
              "Use different models for reading data (queries) and updating data (commands). Queries return DTOs optimized for the UI; commands use the domain model for business logic.",
          },
          {
            id: "arch-cqrs-2",
            question: "When is CQRS useful?",
            answer:
              "When read and write patterns are very different (complex queries vs simple commands, or vice versa), or when you need independent scaling of reads and writes.",
          },
        ],
      },
      {
        id: "hexagonal",
        name: "Hexagonal Architecture",
        summary:
          "Ports and adapters pattern — the application core exposes ports, and adapters connect external systems through them.",
        flashcards: [
          {
            id: "arch-hex-1",
            question:
              "What is the difference between a Port and an Adapter?",
            answer:
              "A Port is an interface defined by the application (e.g., Repository port). An Adapter is the implementation connecting to a specific technology (e.g., PostgresAdapter, InMemoryAdapter).",
          },
          {
            id: "arch-hex-2",
            question: "Why is it called 'hexagonal'?",
            answer:
              "The hexagon shape visually shows multiple ports on different sides — not because there are exactly six. It emphasizes that the application can have any number of inbound and outbound adapters.",
          },
        ],
      },
    ],
  },
  {
    id: "javascript",
    name: "JavaScript Fundamentals",
    description:
      "Core language concepts, patterns, and runtime behavior in modern JavaScript.",
    concepts: [
      {
        id: "closures",
        name: "Closures",
        summary:
          "A function that retains access to its lexical scope even when executed outside its original context.",
        flashcards: [
          {
            id: "js-cl-1",
            question: "What is a closure?",
            answer:
              "A closure is a function combined with its lexical environment. The function remembers and can access variables from its outer scope even after the outer function has returned.",
          },
          {
            id: "js-cl-2",
            question: "Give a practical use case for closures.",
            answer:
              "Module pattern, data privacy (creating private variables), partial application/currying, event handlers that need to reference outer state, and maintaining state in callbacks.",
          },
          {
            id: "js-cl-3",
            question: "What happens to the outer variables after the outer function returns?",
            answer:
              "They are NOT garbage collected if any inner function still holds a reference to them. The closure keeps the entire lexical environment alive.",
          },
        ],
      },
      {
        id: "promises",
        name: "Promises",
        summary:
          "Objects representing the eventual completion (or failure) of an asynchronous operation.",
        flashcards: [
          {
            id: "js-pr-1",
            question: "What are the three states of a Promise?",
            answer:
              "Pending (initial), Fulfilled (operation completed successfully), and Rejected (operation failed). Once settled, the state cannot change.",
          },
          {
            id: "js-pr-2",
            question: "What is the difference between Promise.all and Promise.allSettled?",
            answer:
              "Promise.all fails fast — if any promise rejects, the whole thing rejects immediately. Promise.allSettled waits for all promises to settle (fulfill or reject) and returns results for each.",
          },
        ],
      },
      {
        id: "event-loop",
        name: "Event Loop",
        summary:
          "The mechanism that allows JavaScript to perform non-blocking I/O operations despite being single-threaded.",
        flashcards: [
          {
            id: "js-el-1",
            question: "How does the Event Loop work at a high level?",
            answer:
              "It continuously checks the call stack. When empty, it processes microtasks (Promise callbacks) before macrotasks (setTimeout, I/O). Each cycle: one macrotask, then all microtasks.",
          },
          {
            id: "js-el-2",
            question: "What runs first: setTimeout(fn, 0) or Promise.resolve().then(fn)?",
            answer:
              "Promise.resolve().then(fn) runs first because microtasks are processed before the next macrotask. setTimeout is a macrotask; Promise.then is a microtask.",
          },
        ],
      },
    ],
  },
];

export const MOCK_METRICS: StudyMetrics = {
  dailyCardCount: 45,
  retentionRate: 0.78,
  currentStreak: 12,
  totalReviewed: 280,
};
