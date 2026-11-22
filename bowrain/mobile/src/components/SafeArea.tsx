// The safe-area view with className support. Uniwind styles React Native's
// own components; a third-party component takes classes through withUniwind.
import { SafeAreaView as RNSafeAreaView } from "react-native-safe-area-context";
import { withUniwind } from "uniwind";

export const SafeAreaView = withUniwind(RNSafeAreaView);
