import { useMemo } from 'react';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { issues } from '@/data/issues';
import { useFilter } from '@/context/FilterContext';

export default function IssuesFeed() {
  const { workspace, agent, search } = useFilter();

  const filtered = useMemo(() => {
    let result = issues;
    if (workspace) result = result.filter((iss) => iss.workspace === workspace);
    if (agent) result = result.filter((iss) => iss.agentId === agent);
    if (search) {
      const q = search.toLowerCase();
      result = result.filter((iss) => iss.title.toLowerCase().includes(q));
    }
    return result;
  }, [workspace, agent, search]);

  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        GitHub issues from agent-feedback repo
      </p>

      <div className="rounded-lg border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Title</TableHead>
              <TableHead>Labels</TableHead>
              <TableHead className="hidden sm:table-cell">Filed By</TableHead>
              <TableHead>Status</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {filtered.length === 0 ? (
              <TableRow>
                <TableCell colSpan={4} className="py-8 text-center text-muted-foreground">
                  No issues found.
                </TableCell>
              </TableRow>
            ) : (
              filtered.map((issue) => (
                <TableRow key={issue.id}>
                  <TableCell className="max-w-[200px] text-xs font-medium">
                    {issue.title}
                  </TableCell>
                  <TableCell>
                    <div className="flex flex-wrap gap-1">
                      {issue.labels.map((label) => (
                        <Badge
                          key={label.name}
                          variant="outline"
                          className="text-[10px]"
                          style={{
                            borderColor: `${label.color}60`,
                            color: label.color,
                          }}
                        >
                          {label.name}
                        </Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell className="hidden text-xs text-muted-foreground sm:table-cell">
                    {issue.agentName}
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={issue.status === 'open' ? 'default' : 'secondary'}
                      className="text-xs"
                    >
                      {issue.status}
                    </Badge>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </div>
    </div>
  );
}
