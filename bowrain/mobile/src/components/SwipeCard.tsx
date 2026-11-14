// The top card of the deck, moved by a pan gesture on the UI thread. Right
// approves, left rejects, up puts the card at the end of the deck. While the
// card moves it tilts with the drag and a stamp fades in for the direction;
// crossing the commit distance gives a haptic tick, and a release past it
// throws the card off the screen. The buttons below the card call the same
// actions through the ref, so the card leaves the same way whichever was used.
import * as Haptics from "expo-haptics";
import { useEffect, useImperativeHandle, type ReactNode, type Ref } from "react";
import { Text, useWindowDimensions, View, type AccessibilityActionEvent } from "react-native";
import { GestureDetector, usePanGesture } from "react-native-gesture-handler";
import Animated, {
  Extrapolation,
  interpolate,
  useAnimatedReaction,
  useAnimatedStyle,
  useReducedMotion,
  useSharedValue,
  withSpring,
  withTiming,
  type SharedValue,
} from "react-native-reanimated";
import { scheduleOnRN } from "react-native-worklets";
import { useTranslation } from "react-i18next";

export type SwipeAction = "approve" | "reject" | "fix" | "later";

/** The side a card leaves by, and comes back from on undo. */
export type SwipeSide = "left" | "right" | "up";

export interface SwipeCardHandle {
  /** Move the card off the screen for an action, then run it. */
  swipe(action: SwipeAction): void;
  /** Bring the card back from the side it left by. */
  restore(from: SwipeSide): void;
}

export interface SwipeCardProps {
  children: ReactNode;
  /** Whether the approve swipe is offered for this card. */
  canApprove: boolean;
  onAction: (action: SwipeAction) => void;
  /** Slide the card in from this side when it mounts (an undone decision). */
  enterFrom?: SwipeSide | null;
  /**
   * How far the card is from its resting place, 0 to 1, for the cards behind
   * it to rise as it leaves.
   */
  progress?: SharedValue<number>;
  ref?: Ref<SwipeCardHandle>;
}

/** How far a card moves, as a share of the screen width, before a release commits it. */
const COMMIT_SHARE = 0.3;
const SPRING = { damping: 18, stiffness: 180, mass: 0.9 } as const;
const FLY_MS = 220;

const tick = () => void Haptics.selectionAsync();

export function SwipeCard({
  children,
  canApprove,
  onAction,
  enterFrom,
  progress,
  ref,
}: SwipeCardProps) {
  const { t } = useTranslation();
  const { width, height } = useWindowDimensions();
  const reduceMotion = useReducedMotion();
  const offX = width * 1.5;
  const offY = height * 1.2;
  const commitX = width * COMMIT_SHARE;
  const commitY = height * 0.18;

  const start = (side: SwipeSide | null | undefined) =>
    side === "left" ? -offX : side === "right" ? offX : 0;
  const tx = useSharedValue(reduceMotion ? 0 : start(enterFrom));
  const ty = useSharedValue(!reduceMotion && enterFrom === "up" ? -offY : 0);
  const opacity = useSharedValue(reduceMotion && enterFrom ? 0 : 1);
  // Past the commit distance in the current drag, for the haptic tick.
  const over = useSharedValue(false);

  useEffect(() => {
    if (!enterFrom) return;
    tx.value = withSpring(0, SPRING);
    ty.value = withSpring(0, SPRING);
    opacity.value = withTiming(1, { duration: 160 });
    // Mount only: a card enters once.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useAnimatedReaction(
    () => Math.min(1, Math.max(Math.abs(tx.value) / commitX, Math.abs(ty.value) / commitY)),
    (p) => {
      if (progress) progress.value = p;
    },
  );

  const run = (action: SwipeAction) => onAction(action);

  const leave = (action: SwipeAction) => {
    "worklet";
    if (action === "fix") {
      // The fix sheet opens over the card, which stays in its place.
      tx.value = withSpring(0, SPRING);
      ty.value = withSpring(0, SPRING);
      scheduleOnRN(run, action);
      return;
    }
    const done = (finished?: boolean) => {
      "worklet";
      if (finished) scheduleOnRN(run, action);
    };
    if (reduceMotion) {
      opacity.value = withTiming(0, { duration: 120 }, done);
      return;
    }
    if (action === "later") ty.value = withTiming(-offY, { duration: FLY_MS }, done);
    else tx.value = withTiming(action === "approve" ? offX : -offX, { duration: FLY_MS }, done);
  };

  const restore = (from: SwipeSide) => {
    if (reduceMotion) {
      tx.value = 0;
      ty.value = 0;
      opacity.value = withTiming(1, { duration: 160 });
      return;
    }
    opacity.value = 1;
    if (from === "up") ty.value = -offY;
    else tx.value = from === "left" ? -offX : offX;
    tx.value = withSpring(0, SPRING);
    ty.value = withSpring(0, SPRING);
  };

  // From the JS thread: the fix sheet needs no animation, so it opens at once.
  const swipe = (action: SwipeAction) => (action === "fix" ? run(action) : leave(action));

  useImperativeHandle(ref, () => ({ swipe, restore }));

  const pan = usePanGesture({
    activeOffsetX: [-12, 12],
    activeOffsetY: -20,
    onUpdate: (e) => {
      tx.value = canApprove ? e.translationX : Math.min(e.translationX, 40);
      ty.value = Math.min(e.translationY, 0) * 0.9;
      const past =
        (canApprove && e.translationX > commitX) ||
        e.translationX < -commitX ||
        (e.translationY < -commitY && Math.abs(e.translationX) < commitX);
      if (past !== over.value) {
        over.value = past;
        if (past) scheduleOnRN(tick);
      }
    },
    onDeactivate: (e) => {
      over.value = false;
      // A quick flick commits short of the distance.
      const flingX = Math.abs(e.velocityX) > 900 && Math.abs(e.translationX) > commitX * 0.4;
      if (canApprove && (e.translationX > commitX || (flingX && e.velocityX > 0))) {
        leave("approve");
      } else if (e.translationX < -commitX || (flingX && e.velocityX < 0)) {
        leave("reject");
      } else if (e.translationY < -commitY && Math.abs(e.translationX) < commitX) {
        leave("later");
      } else {
        tx.value = withSpring(0, SPRING);
        ty.value = withSpring(0, SPRING);
      }
    },
  });

  const cardStyle = useAnimatedStyle(() => ({
    opacity: opacity.value,
    transform: [
      { translateX: tx.value },
      { translateY: ty.value },
      {
        rotate: reduceMotion
          ? "0deg"
          : `${interpolate(tx.value, [-width, 0, width], [-14, 0, 14])}deg`,
      },
    ],
  }));
  const approveStyle = useAnimatedStyle(() => ({
    opacity: interpolate(tx.value, [commitX * 0.15, commitX], [0, 1], Extrapolation.CLAMP),
  }));
  const rejectStyle = useAnimatedStyle(() => ({
    opacity: interpolate(tx.value, [-commitX, -commitX * 0.15], [1, 0], Extrapolation.CLAMP),
  }));
  const approveTint = useAnimatedStyle(() => ({
    opacity: interpolate(tx.value, [0, commitX], [0, 0.14], Extrapolation.CLAMP),
  }));
  const rejectTint = useAnimatedStyle(() => ({
    opacity: interpolate(tx.value, [-commitX, 0], [0.14, 0], Extrapolation.CLAMP),
  }));
  const laterStyle = useAnimatedStyle(() => ({
    opacity:
      interpolate(ty.value, [-commitY, -commitY * 0.2], [1, 0], Extrapolation.CLAMP) *
      interpolate(Math.abs(tx.value), [0, commitX * 0.5], [1, 0], Extrapolation.CLAMP),
  }));

  const onAccessibilityAction = (e: AccessibilityActionEvent) => {
    const name = e.nativeEvent.actionName;
    if (name === "approve" || name === "reject" || name === "fix" || name === "later") {
      swipe(name);
    }
  };

  return (
    <GestureDetector gesture={pan}>
      <Animated.View
        style={[{ flex: 1 }, cardStyle]}
        accessibilityActions={[
          ...(canApprove ? [{ name: "approve", label: t("actions.approve") }] : []),
          { name: "reject", label: t("actions.reject") },
          { name: "fix", label: t("actions.fix") },
          { name: "later", label: t("actions.later") },
        ]}
        onAccessibilityAction={onAccessibilityAction}
      >
        {children}
        <Animated.View
          pointerEvents="none"
          className="bg-approve absolute inset-0 rounded-[28px]"
          style={approveTint}
        />
        <Animated.View
          pointerEvents="none"
          className="bg-reject absolute inset-0 rounded-[28px]"
          style={rejectTint}
        />
        <Stamp
          testID="stamp-approve"
          label={t("swipe.approve")}
          className="border-approve left-5"
          textClassName="text-approve"
          rotate="-14deg"
          style={approveStyle}
        />
        <Stamp
          testID="stamp-reject"
          label={t("swipe.reject")}
          className="border-reject right-5"
          textClassName="text-reject"
          rotate="14deg"
          style={rejectStyle}
        />
        <Animated.View
          pointerEvents="none"
          className="absolute inset-x-0 bottom-8 items-center"
          style={laterStyle}
        >
          <View className="border-muted-foreground rounded-xl border-4 px-4 py-1">
            <Text className="text-muted-foreground text-2xl font-extrabold tracking-widest uppercase">
              {t("swipe.later")}
            </Text>
          </View>
        </Animated.View>
      </Animated.View>
    </GestureDetector>
  );
}

function Stamp({
  label,
  className,
  textClassName,
  rotate,
  style,
  testID,
}: {
  label: string;
  className: string;
  textClassName: string;
  rotate: string;
  style: ReturnType<typeof useAnimatedStyle>;
  testID: string;
}) {
  return (
    <Animated.View
      testID={testID}
      pointerEvents="none"
      className={`bg-card absolute top-8 rounded-xl border-4 px-4 py-1 ${className}`}
      style={[{ transform: [{ rotate }] }, style]}
    >
      <Text className={`text-3xl font-extrabold tracking-widest uppercase ${textClassName}`}>
        {label}
      </Text>
    </Animated.View>
  );
}

/**
 * A card waiting behind the top one, `depth` places down. The deck shows two
 * or three, each a little smaller and lower, and they rise one place as the
 * top card leaves.
 */
export function StackedCard({
  depth,
  progress,
  children,
}: {
  depth: number;
  progress: SharedValue<number>;
  children: ReactNode;
}) {
  const style = useAnimatedStyle(() => {
    const pos = depth - progress.value;
    return {
      opacity: interpolate(pos, [0, 2, 3], [1, 1, 0], Extrapolation.CLAMP),
      transform: [{ translateY: 12 * pos }, { scale: 1 - 0.05 * pos }],
    };
  });
  return (
    <Animated.View
      pointerEvents="none"
      accessibilityElementsHidden
      importantForAccessibility="no-hide-descendants"
      className="absolute inset-0"
      style={[{ transformOrigin: "bottom" }, style]}
    >
      {children}
    </Animated.View>
  );
}
