import React from "react";
import { View, Text, StyleSheet, Dimensions, TouchableOpacity } from "react-native";
import { Gesture, GestureDetector } from "react-native-gesture-handler";
import Animated, {
  useSharedValue,
  useAnimatedStyle,
  withSpring,
  runOnJS,
  interpolate,
} from "react-native-reanimated";
import type { ReviewItem } from "../api/client";

const SCREEN_WIDTH = Dimensions.get("window").width;
const SWIPE_THRESHOLD = SCREEN_WIDTH * 0.3;

interface SwipeCardProps {
  item: ReviewItem;
  onApprove: () => void;
  onReject: () => void;
  onSkip: () => void;
}

/** Extract display info from review item data. */
function itemTitle(item: ReviewItem): string {
  return (item.data as any)?.text ?? (item.data as any)?.Text ?? "Unknown";
}

function itemType(item: ReviewItem): string {
  if (item.type === "term_candidate") return "Term Candidate";
  if (item.type === "entity_review") return "Entity";
  return item.type;
}

function itemDetails(item: ReviewItem): string[] {
  const details: string[] = [];
  const d = item.data as any;
  if (d?.definition) details.push(d.definition);
  if (d?.category) details.push(`Category: ${d.category}`);
  if (d?.type) details.push(`Type: ${d.type}`);
  if (d?.translatability) details.push(`Translatability: ${d.translatability}`);
  return details;
}

export function SwipeCard({ item, onApprove, onReject, onSkip }: SwipeCardProps) {
  const translateX = useSharedValue(0);

  const pan = Gesture.Pan()
    .onUpdate((e) => {
      translateX.value = e.translationX;
    })
    .onEnd((e) => {
      if (e.translationX > SWIPE_THRESHOLD) {
        translateX.value = withSpring(SCREEN_WIDTH);
        runOnJS(onApprove)();
      } else if (e.translationX < -SWIPE_THRESHOLD) {
        translateX.value = withSpring(-SCREEN_WIDTH);
        runOnJS(onReject)();
      } else {
        translateX.value = withSpring(0);
      }
    });

  const animatedStyle = useAnimatedStyle(() => ({
    transform: [
      { translateX: translateX.value },
      {
        rotate: `${interpolate(translateX.value, [-SCREEN_WIDTH, 0, SCREEN_WIDTH], [-15, 0, 15])}deg`,
      },
    ],
    opacity: interpolate(Math.abs(translateX.value), [0, SCREEN_WIDTH * 0.5], [1, 0.5]),
  }));

  const approveOpacity = useAnimatedStyle(() => ({
    opacity: interpolate(translateX.value, [0, SWIPE_THRESHOLD], [0, 1]),
  }));

  const rejectOpacity = useAnimatedStyle(() => ({
    opacity: interpolate(translateX.value, [-SWIPE_THRESHOLD, 0], [1, 0]),
  }));

  const confidence = Math.round(item.confidence * 100);

  return (
    <GestureDetector gesture={pan}>
      <Animated.View style={[styles.card, animatedStyle]}>
        {/* Approve indicator */}
        <Animated.View style={[styles.indicator, styles.approveIndicator, approveOpacity]}>
          <Text style={styles.indicatorText}>APPROVE</Text>
        </Animated.View>

        {/* Reject indicator */}
        <Animated.View style={[styles.indicator, styles.rejectIndicator, rejectOpacity]}>
          <Text style={styles.indicatorText}>REJECT</Text>
        </Animated.View>

        {/* Card content */}
        <View style={styles.header}>
          <Text style={styles.type}>{itemType(item)}</Text>
          <Text style={styles.confidence}>{confidence}%</Text>
        </View>

        <Text style={styles.title}>{itemTitle(item)}</Text>

        {itemDetails(item).map((detail, i) => (
          <Text key={i} style={styles.detail}>
            {detail}
          </Text>
        ))}

        {/* Occurrences / context */}
        {item.occurrences.length > 0 && (
          <View style={styles.contextSection}>
            <Text style={styles.contextLabel}>
              {item.occurrences.length} occurrence{item.occurrences.length > 1 ? "s" : ""}
            </Text>
            {item.occurrences.slice(0, 2).map((occ, i) => (
              <Text key={i} style={styles.contextText} numberOfLines={2}>
                &ldquo;{occ.context}&rdquo;
              </Text>
            ))}
          </View>
        )}

        {/* Skip button */}
        <TouchableOpacity style={styles.skipButton} onPress={onSkip}>
          <Text style={styles.skipText}>Skip</Text>
        </TouchableOpacity>
      </Animated.View>
    </GestureDetector>
  );
}

const styles = StyleSheet.create({
  card: {
    position: "absolute",
    width: SCREEN_WIDTH - 32,
    backgroundColor: "#161b22",
    borderRadius: 16,
    padding: 24,
    borderWidth: 1,
    borderColor: "#30363d",
    shadowColor: "#000",
    shadowOffset: { width: 0, height: 4 },
    shadowOpacity: 0.3,
    shadowRadius: 8,
    elevation: 8,
  },
  indicator: {
    position: "absolute",
    top: 16,
    paddingHorizontal: 12,
    paddingVertical: 6,
    borderRadius: 8,
    borderWidth: 2,
  },
  approveIndicator: {
    right: 16,
    borderColor: "#3fb950",
  },
  rejectIndicator: {
    left: 16,
    borderColor: "#f85149",
  },
  indicatorText: {
    fontWeight: "800",
    fontSize: 14,
    letterSpacing: 1,
    color: "#e6edf3",
  },
  header: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    marginBottom: 12,
  },
  type: {
    fontSize: 12,
    fontWeight: "600",
    color: "#c27930",
    textTransform: "uppercase",
    letterSpacing: 0.5,
  },
  confidence: {
    fontSize: 12,
    fontWeight: "700",
    color: "#8b949e",
    backgroundColor: "#21262d",
    paddingHorizontal: 8,
    paddingVertical: 2,
    borderRadius: 10,
    overflow: "hidden",
  },
  title: {
    fontSize: 22,
    fontWeight: "700",
    color: "#e6edf3",
    marginBottom: 8,
  },
  detail: {
    fontSize: 14,
    color: "#8b949e",
    marginBottom: 4,
  },
  contextSection: {
    marginTop: 16,
    paddingTop: 12,
    borderTopWidth: 1,
    borderTopColor: "#30363d",
  },
  contextLabel: {
    fontSize: 11,
    fontWeight: "600",
    color: "#8b949e",
    textTransform: "uppercase",
    letterSpacing: 0.5,
    marginBottom: 8,
  },
  contextText: {
    fontSize: 13,
    color: "#8b949e",
    fontStyle: "italic",
    marginBottom: 4,
  },
  skipButton: {
    marginTop: 16,
    alignSelf: "center",
    paddingHorizontal: 20,
    paddingVertical: 8,
    borderRadius: 8,
    backgroundColor: "#21262d",
  },
  skipText: {
    color: "#8b949e",
    fontSize: 14,
    fontWeight: "600",
  },
});
