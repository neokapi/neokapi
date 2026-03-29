// Shared UI primitives — used by both framework (kapi-desktop) and platform (bowrain).
// This package contains only framework-safe components with zero platform dependencies.

// Utility
export { cn } from "./lib/utils";

// shadcn/ui primitives
export { Button, buttonVariants } from "./components/ui/button";
export {
  Card,
  CardHeader,
  CardTitle,
  CardAction,
  CardDescription,
  CardContent,
  CardFooter,
} from "./components/ui/card";
export { Input } from "./components/ui/input";
export { Label } from "./components/ui/label";
export { Badge, badgeVariants } from "./components/ui/badge";
export { Separator } from "./components/ui/separator";
export { Tabs, TabsList, TabsTrigger, TabsContent } from "./components/ui/tabs";
export {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "./components/ui/select";
export { Switch } from "./components/ui/switch";
export {
  Collapsible,
  CollapsibleTrigger,
  CollapsibleContent,
} from "./components/ui/collapsible";
export {
  Dialog,
  DialogTrigger,
  DialogContent,
  DialogHeader,
  DialogFooter,
  DialogTitle,
  DialogDescription,
  DialogClose,
} from "./components/ui/dialog";
export { Skeleton } from "./components/ui/skeleton";
export { Textarea } from "./components/ui/textarea";
export { TooltipProvider } from "./components/ui/tooltip";

// Filter config editor (dynamic forms from JSON schema)
export {
  FilterConfigEditor,
  SchemaConfigEditor,
} from "./components/filter";
export type {
  ComponentSchema,
  FilterSchema,
  ParameterGroup,
  PropertySchema,
  FilterParamsValue,
} from "./components/filter/types";
