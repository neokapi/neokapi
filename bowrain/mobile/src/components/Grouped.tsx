// The grouped list of a settings screen, as iOS draws one: rounded sections
// on the page background, each row with an icon tile, a title, a value and a
// chevron, and a hairline between rows that starts after the icon.
import Ionicons from "@expo/vector-icons/Ionicons";
import { Children, Fragment, isValidElement, type ComponentProps, type ReactNode } from "react";
import { Pressable, Text, View } from "react-native";
import { useCSSVariable } from "uniwind";

type IconName = ComponentProps<typeof Ionicons>["name"];

/** A section: an optional header above it and footer below it. */
export function Section({
  header,
  footer,
  children,
}: {
  header?: string;
  footer?: string;
  children: ReactNode;
}) {
  const rows = Children.toArray(children).filter(isValidElement<{ icon?: IconName }>);
  return (
    <View className="gap-1.5 px-4">
      {header ? (
        <Text accessibilityRole="header" className="text-muted-foreground px-4 text-sm uppercase">
          {header}
        </Text>
      ) : null}
      <View className="bg-card overflow-hidden rounded-xl">
        {rows.map((row, i) => (
          <Fragment key={row.key ?? i}>
            {i > 0 ? (
              <View className={`bg-border h-px ${row.props.icon ? "ml-14" : "ml-4"}`} />
            ) : null}
            {row}
          </Fragment>
        ))}
      </View>
      {footer ? <Text className="text-muted-foreground px-4 text-sm">{footer}</Text> : null}
    </View>
  );
}

// The icon tiles' colours, spelled out so Tailwind sees every class.
const TILES = {
  blue: "bg-prism-4",
  violet: "bg-prism-5",
  green: "bg-prism-3",
  amber: "bg-prism-2",
  rose: "bg-prism-1",
  grey: "bg-muted-foreground",
  red: "bg-reject",
} as const;

export type Tile = keyof typeof TILES;

/** One row. With `onPress` it is a button; with `selected` it is a choice. */
export function Cell({
  icon,
  tile = "blue",
  title,
  value,
  onPress,
  chevron,
  external,
  selected,
  destructive,
  accessibilityLabel,
  accessibilityHint,
  testID,
}: {
  icon?: IconName;
  tile?: Tile;
  title: string;
  value?: string;
  onPress?: () => void;
  /** A chevron: the row opens another screen. */
  chevron?: boolean;
  /** An arrow out: the row opens the browser. */
  external?: boolean;
  /** A checkmark for the chosen row of a choice; false leaves room for one. */
  selected?: boolean;
  destructive?: boolean;
  accessibilityLabel?: string;
  accessibilityHint?: string;
  testID?: string;
}) {
  const [muted, primary] = useCSSVariable(["--color-muted-foreground", "--color-primary"]);
  const choice = selected !== undefined;
  const content = (
    <>
      {icon ? (
        <View
          accessibilityElementsHidden
          importantForAccessibility="no"
          className={`${TILES[tile]} h-[30px] w-[30px] items-center justify-center rounded-[7px]`}
        >
          <Ionicons name={icon} size={18} color="#ffffff" />
        </View>
      ) : null}
      <Text
        className={`flex-1 text-base ${destructive ? "text-reject" : "text-foreground"} ${
          destructive && !icon ? "text-center" : ""
        }`}
      >
        {title}
      </Text>
      {value ? (
        <Text numberOfLines={1} className="text-muted-foreground max-w-[55%] text-base">
          {value}
        </Text>
      ) : null}
      {choice ? (
        <View className="h-6 w-6 items-center justify-center">
          {selected ? <Ionicons name="checkmark" size={22} color={String(primary)} /> : null}
        </View>
      ) : null}
      {external ? <Ionicons name="open-outline" size={17} color={String(muted)} /> : null}
      {chevron ? <Ionicons name="chevron-forward" size={18} color={String(muted)} /> : null}
    </>
  );
  const className = "min-h-[48px] flex-row items-center gap-3 px-4 py-2.5";
  if (!onPress) {
    return (
      <View
        testID={testID}
        accessible
        accessibilityLabel={accessibilityLabel}
        className={className}
      >
        {content}
      </View>
    );
  }
  return (
    <Pressable
      testID={testID}
      accessibilityRole={choice ? "radio" : external ? "link" : "button"}
      accessibilityState={choice ? { selected } : undefined}
      accessibilityLabel={accessibilityLabel}
      accessibilityHint={accessibilityHint}
      onPress={onPress}
      className={`${className} active:bg-muted`}
    >
      {content}
    </Pressable>
  );
}
