// CommentsPanel — discussion on a concept (Apache-2.0). Threaded comments;
// resolved threads stay as part of the record. Rendered only when the source
// supplies comments. Read-only here — composing/resolving is the consuming
// app's concern (it knows the author and the write path).
import { useMemo } from "react";
import { Skeleton, cn } from "@neokapi/ui-primitives";
import { Check, MessagesSquare } from "lucide-react";
import type { ConceptSectionProps } from "./ConceptView";
import type { Comment } from "./types";
import { ConceptSection, EmptyHint, ErrorHint, formatRelative } from "./atoms";
import { useResource } from "./useResource";

interface ThreadNode {
  comment: Comment;
  replies: ThreadNode[];
}

// Build parent→children threads, preserving source order at each level.
function buildThreads(comments: Comment[]): ThreadNode[] {
  const byId = new Map<string, ThreadNode>();
  comments.forEach((c) => byId.set(c.id, { comment: c, replies: [] }));
  const roots: ThreadNode[] = [];
  comments.forEach((c) => {
    const node = byId.get(c.id)!;
    const parent = c.parentId ? byId.get(c.parentId) : undefined;
    if (parent) parent.replies.push(node);
    else roots.push(node);
  });
  return roots;
}

export function CommentsPanel({ concept, source, capabilities }: ConceptSectionProps) {
  const res = useResource<Comment[]>(
    () => (capabilities.comments && source.getComments ? source.getComments(concept.id) : []),
    [source, concept.id, capabilities.comments],
  );
  const threads = useMemo(() => buildThreads(res.data ?? []), [res.data]);
  const loading = res.loading && !res.data;

  return (
    <ConceptSection
      title="Discussion"
      icon={<MessagesSquare />}
      description="Comments on this concept."
    >
      {res.error ? (
        <ErrorHint title="Could not load discussion" description={res.error.message} />
      ) : loading ? (
        <div className="space-y-2">
          <Skeleton className="h-10 w-full rounded-lg" />
          <Skeleton className="ml-6 h-10 w-3/4 rounded-lg" />
        </div>
      ) : threads.length === 0 ? (
        <EmptyHint
          icon={<MessagesSquare />}
          title="No discussion yet"
          description="Decisions are easier to trust with the conversation kept beside them."
        />
      ) : (
        <ul className="space-y-3">
          {threads.map((node) => (
            <CommentThread key={node.comment.id} node={node} depth={0} />
          ))}
        </ul>
      )}
    </ConceptSection>
  );
}

function CommentThread({ node, depth }: { node: ThreadNode; depth: number }) {
  const { comment, replies } = node;
  return (
    <li className={cn(depth > 0 && "ml-5 border-l pl-3")}>
      <div className="flex items-baseline gap-2">
        <span className="text-sm font-medium text-foreground">{comment.author}</span>
        <span className="text-[11px] text-muted-foreground">{formatRelative(comment.at)}</span>
        {comment.resolved && (
          <span className="inline-flex items-center gap-0.5 text-[11px] text-success">
            <Check className="size-3" />
            resolved
          </span>
        )}
      </div>
      <p className="mt-0.5 text-sm text-foreground">{comment.body}</p>
      {replies.length > 0 && (
        <ul className="mt-2 space-y-2">
          {replies.map((r) => (
            <CommentThread key={r.comment.id} node={r} depth={depth + 1} />
          ))}
        </ul>
      )}
    </li>
  );
}
