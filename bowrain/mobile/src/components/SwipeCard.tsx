// The top card of the deck, moved by a pan gesture on the UI thread. Right
// approves, left opens the fix sheet, up puts the card at the end of the deck.
// The buttons below the card call the same actions through the ref, so the
// card leaves the same way whichever was used.
import { useImperativeHandle, type ReactNode, type Ref } from "react";
import { useWindowDimensions } from "react-native";
import { GestureDetector, usePanGesture } from "react-native-gesture-handler";
import Animated, {
  interpolate,
  useAnimatedStyle,
  useSharedValue,
  withSpring,
  withTiming,
} from "react-native-reanimated";
import { scheduleOnRN } from "react-native-worklets";

export type SwipeAction = "approve" | "fix" | "later";

export interface SwipeCardHandle {
  /** Move the card off the screen for an action, then run it. */
  swipe(action: SwipeAction): void;
}

export interface SwipeCardProps {
  children: ReactNode;
  /** Whether the approve swipe is offered for this card. */
  canApprove: boolean;
  onAction: (action: SwipeAction) => void;
  ref?: Ref<SwipeCardHandle>;
}

/** How far a card moves, as a share of the screen width, before a release commits it. */
const COMMIT_SHARE = 0.28;

export function SwipeCard({ children, canApprove, onAction, ref }: SwipeCardProps) {
  const { width, height } = useWindowDimensions();
  const tx = useSharedValue(0);
  const ty = useSharedValue(0);
  const commitX = width * COMMIT_SHARE;
  const commitY = height * 0.18;

  const run = (action: SwipeAction) => onAction(action);

  const leave = (action: SwipeAction) => {
    "worklet";
    if (action === "approve") {
      tx.value = withTiming(width * 1.4, { duration: 180 }, () => scheduleOnRN(run, action));
    } else if (action === "later") {
      ty.value = withTiming(-height, { duration: 200 }, () => scheduleOnRN(run, action));
    } else {
      // The fix sheet opens over the card, which returns to its place.
      tx.value = withSpring(0);
      ty.value = withSpring(0);
      scheduleOnRN(run, action);
    }
  };

  useImperativeHandle(ref, () => ({
    swipe: (action) => {
      if (action === "approve") {
        tx.value = withTiming(width * 1.4, { duration: 180 }, () => scheduleOnRN(run, action));
      } else if (action === "later") {
        ty.value = withTiming(-height, { duration: 200 }, () => scheduleOnRN(run, action));
      } else {
        run(action);
      }
    },
  }));

  const pan = usePanGesture({
    activeOffsetX: [-16, 16],
    activeOffsetY: -24,
    onUpdate: (e) => {
      tx.value = canApprove ? e.translationX : Math.min(e.translationX, 40);
      ty.value = Math.min(e.translationY, 0);
    },
    onDeactivate: (e) => {
      if (canApprove && e.translationX > commitX) leave("approve");
      else if (e.translationX < -commitX) leave("fix");
      else if (e.translationY < -commitY && Math.abs(e.translationX) < commitX) leave("later");
      else {
        tx.value = withSpring(0);
        ty.value = withSpring(0);
      }
    },
  });

  const style = useAnimatedStyle(() => ({
    transform: [
      { translateX: tx.value },
      { translateY: ty.value },
      { rotate: `${interpolate(tx.value, [-width, 0, width], [-12, 0, 12])}deg` },
    ],
  }));

  return (
    <GestureDetector gesture={pan}>
      <Animated.View style={[{ flex: 1 }, style]}>{children}</Animated.View>
    </GestureDetector>
  );
}
