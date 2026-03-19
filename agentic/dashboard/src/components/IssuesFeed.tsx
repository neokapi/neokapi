import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Badge } from '@/components/ui/badge';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { ScrollArea } from '@/components/ui/scroll-area';
import { issues } from '../data/issues';
import { useFilter } from '../context/FilterContext';

export default function IssuesFeed() {
  const { filters } = useFilter();

  const filtered = filters.workspace
    ? issues.filter((iss) => iss.workspace === filters.workspace)
    : issues;

  return (
    <Card className="flex h-full flex-col">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-semibold">Agent-Filed Issues</CardTitle>
      </CardHeader>
      <CardContent className="flex-1 p-0">
        <ScrollArea className="h-[500px]">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Title</TableHead>
                <TableHead>Labels</TableHead>
                <TableHead>Filed By</TableHead>
                <TableHead>Status</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {filtered.map((issue) => (
                <TableRow key={issue.id}>
                  <TableCell className="text-xs font-medium">{issue.title}</TableCell>
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
                  <TableCell className="text-xs text-muted-foreground">
                    {issue.agentName}
                  </TableCell>
                  <TableCell>
                    <Badge variant={issue.status === 'open' ? 'default' : 'secondary'}>
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
