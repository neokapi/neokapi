import { ScrollArea } from '@/components/ui/scroll-area';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { issues } from '@/data/issues';
import { useFilter } from '@/context/FilterContext';

export default function IssuesFeed() {
  const { workspace, agent } = useFilter();

  let filtered = issues;
  if (workspace) filtered = filtered.filter((iss) => iss.workspace === workspace);
  if (agent) filtered = filtered.filter((iss) => iss.agentId === agent);

  return (
    <Card className="flex h-full flex-col">
      <CardHeader>
        <CardTitle className="text-sm">Agent-Filed Issues</CardTitle>
      </CardHeader>
      <CardContent className="flex-1 p-0">
        <ScrollArea className="h-[500px]">
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
              {filtered.map((issue) => (
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
              ))}
            </TableBody>
          </Table>
        </ScrollArea>
      </CardContent>
    </Card>
  );
}
