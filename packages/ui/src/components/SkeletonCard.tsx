import { Card } from "./ui/card";
import { Skeleton } from "./ui/skeleton";

interface SkeletonCardProps {
  lines?: number;
  className?: string;
}

export function SkeletonCard({ lines = 3, className }: SkeletonCardProps) {
  return (
    <Card className={`p-4 ${className ?? ""}`}>
      <Skeleton className="mb-2 h-4 w-2/3" />
      {Array.from({ length: lines - 1 }).map((_, i) => (
        <Skeleton key={i} className={`mt-2 h-3 ${i === lines - 2 ? "w-1/3" : "w-full"}`} />
      ))}
    </Card>
  );
}
