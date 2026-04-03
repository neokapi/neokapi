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

interface EntityReviewCardProps {
  item: ReviewItem;
  onApprove: () => void;
  onReject: () => void;
  onSkip: () => void;
}

/** Map entity type to display color. */
function entityColor(type: string): string {
  switch (type) {
    case "person":
      return "#f85149";
    case "organization":
      return "#58a6ff";
    case "location":
      return "#3fb950";
    case "date":
      return "#d29922";
    case "product":
      return "#bc8cff";
    default:
      return "#8b949e";
  }
}

function entityLabel(type: string): string {
  switch (type) {
    case "person":
      return "Person";
    case "organization":
      return "Organization";
    case "location":
      return "Location";
    case "date":
      return "Date";
    case "product":
      return "Product";
    default:
      return type;
  }
}

export function EntityReviewCard({ item, onApprove, onReject, onSkip }: EntityReviewCardProps) {
  const translateX = useSharedValue(0);
  const data = item.data as Record<string, unknown>;

  const entityText = (data.text as string) ?? (data.Text as string) ?? "Unknown";
  const entityType = (data.type as string) ?? "entity";
  const dnt = (data.dnt as boolean) ?? false;
  const definition = data.definition as string | undefined;
  const confidence = Math.round(item.confidence * 100);

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

  const typeColor = entityColor(entityType);

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

        {/* Entity type badge */}
        <View style={styles.header}>
          <View
            style={[
              styles.typeBadge,
              { backgroundColor: typeColor + "22", borderColor: typeColor },
            ]}
          >
            <Text style={[styles.typeBadgeText, { color: typeColor }]}>
              {entityLabel(entityType)}
            </Text>
          </View>
          <Text style={styles.confidence}>{confidence}%</Text>
        </View>

        {/* Entity text */}
        <Text style={styles.entityText}>{entityText}</Text>

        {/* DNT flag */}
        {dnt && (
          <View style={styles.dntBadge}>
            <Text style={styles.dntText}>Do Not Translate</Text>
          </View>
        )}

        {/* Definition */}
        {definition && <Text style={styles.definition}>{definition}</Text>}

        {/* Occurrences */}
        {item.occurrences.length > 0 && (
          <View style={styles.contextSection}>
            <Text style={styles.contextLabel}>
              {item.occurrences.length} occurrence{item.occurrences.length > 1 ? "s" : ""}
            </Text>
            {item.occurrences.slice(0, 3).map((occ, i) => (
              <Text key={i} style={styles.contextText} numberOfLines={2}>
                &ldquo;...{occ.context}...&rdquo;
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
    marginBottom: 16,
  },
  typeBadge: {
    paddingHorizontal: 10,
    paddingVertical: 4,
    borderRadius: 6,
    borderWidth: 1,
  },
  typeBadgeText: {
    fontSize: 12,
    fontWeight: "700",
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
  entityText: {
    fontSize: 26,
    fontWeight: "800",
    color: "#e6edf3",
    marginBottom: 8,
  },
  dntBadge: {
    alignSelf: "flex-start",
    backgroundColor: "#d2992222",
    borderWidth: 1,
    borderColor: "#d29922",
    borderRadius: 6,
    paddingHorizontal: 8,
    paddingVertical: 3,
    marginBottom: 8,
  },
  dntText: {
    fontSize: 11,
    fontWeight: "700",
    color: "#d29922",
    textTransform: "uppercase",
    letterSpacing: 0.5,
  },
  definition: {
    fontSize: 14,
    color: "#8b949e",
    marginBottom: 8,
    lineHeight: 20,
  },
  contextSection: {
    marginTop: 12,
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
