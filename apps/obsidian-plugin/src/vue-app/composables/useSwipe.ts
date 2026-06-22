import { onMounted, onUnmounted, type Ref } from "vue";

interface SwipeCallbacks {
  onSwipeLeft?: () => void;
  onSwipeRight?: () => void;
}

/**
 * Composable that attaches touch event listeners to an element
 * and fires callbacks when a horizontal swipe exceeds the threshold.
 * Uses passive listeners to avoid blocking scroll. Minimum 44×44px touch area.
 */
export function useSwipe(
  el: Ref<HTMLElement | null>,
  callbacks: SwipeCallbacks,
  threshold = 60,
): void {
  let startX = 0;
  let startY = 0;

  function handleTouchStart(e: TouchEvent) {
    if (e.touches.length !== 1) return;
    startX = e.touches[0].clientX;
    startY = e.touches[0].clientY;
  }

  function handleTouchEnd(e: TouchEvent) {
    if (e.changedTouches.length !== 1) return;
    const endX = e.changedTouches[0].clientX;
    const endY = e.changedTouches[0].clientY;
    const deltaX = endX - startX;
    const deltaY = endY - startY;

    // Only trigger if horizontal swipe dominates vertical
    if (Math.abs(deltaX) < Math.abs(deltaY)) return;
    if (Math.abs(deltaX) < threshold) return;

    if (deltaX > 0 && callbacks.onSwipeRight) {
      callbacks.onSwipeRight();
    } else if (deltaX < 0 && callbacks.onSwipeLeft) {
      callbacks.onSwipeLeft();
    }
  }

  onMounted(() => {
    const element = el.value;
    if (!element) return;

    element.addEventListener("touchstart", handleTouchStart, {
      passive: true,
    });
    element.addEventListener("touchend", handleTouchEnd, { passive: true });
  });

  onUnmounted(() => {
    const element = el.value;
    if (!element) return;

    element.removeEventListener("touchstart", handleTouchStart);
    element.removeEventListener("touchend", handleTouchEnd);
  });
}
