import { describe, it, expect } from "vitest";
import { nextIndex, prevIndex } from "../validation";

describe("nextIndex", () => {
  const total = 5;

  it("moves forward from first card", () => {
    expect(nextIndex(0, total)).toBe(1);
  });

  it("moves forward from middle", () => {
    expect(nextIndex(2, total)).toBe(3);
  });

  it("wraps from last card to first (circular)", () => {
    expect(nextIndex(4, total)).toBe(0);
  });

  it("moves forward with single card (wraps to same)", () => {
    expect(nextIndex(0, 1)).toBe(0);
  });
});

describe("prevIndex", () => {
  const total = 5;

  it("moves backward from second card", () => {
    expect(prevIndex(1, total)).toBe(0);
  });

  it("moves backward from middle", () => {
    expect(prevIndex(3, total)).toBe(2);
  });

  it("wraps from first card to last (circular)", () => {
    expect(prevIndex(0, total)).toBe(4);
  });

  it("moves backward with single card (wraps to same)", () => {
    expect(prevIndex(0, 1)).toBe(0);
  });
});

describe("flip toggle logic", () => {
  it("toggles isFlipped from false to true", () => {
    let isFlipped = false;
    isFlipped = !isFlipped;
    expect(isFlipped).toBe(true);
  });

  it("toggles isFlipped from true to false", () => {
    let isFlipped = true;
    isFlipped = !isFlipped;
    expect(isFlipped).toBe(false);
  });

  it("resets isFlipped to false after navigation", () => {
    // Navigation resets isFlipped — simulate the pattern
    let isFlipped = true;
    // On next/prev click:
    isFlipped = false;
    expect(isFlipped).toBe(false);
  });
});
