export interface ConstraintScope {
  locale?: string;
  channel?: string;
  persona?: string;
}

export interface ConstraintException {
  scope: ConstraintScope;
  reason: string;
  approved_by: string;
  approval_ref: string;
}

export interface ConstraintResolution {
  constraint: {
    id: string;
    version: number;
    source: string;
    statement: string;
    kind: string;
    regex?: string;
    exceptions?: ConstraintException[];
  };
  status: string;
}

export interface RecordedCase {
  id: string;
  audience: string;
  file: string;
  input: Record<string, string>;
  inputSha256: string;
  actualConstraintFindings: number;
  passed: boolean;
  resolvedContext: {
    point: { coordinates?: Record<string, string> };
    constraints?: ConstraintResolution[];
    voice?: { guide: string };
  };
  run: { command: string[]; exitCode: number; stdout: string; stderr: string };
  contextRun: {
    command: string[];
    exitCode: number;
    stdout: string;
    stderr: string;
  };
  report: {
    pass: boolean;
    summary: { findings: number; score: number };
    findings: Array<{
      severity: string;
      message: string;
      metadata?: Record<string, string>;
    }>;
    execution?: {
      analyzers: Array<{
        id: string;
        status: string;
        required: boolean;
        findings: number;
        reason?: string;
      }>;
    };
  };
  semanticVerification: string;
}

export interface AudienceEvidence {
  schema: string;
  failure?: { message: string; case?: unknown; attempt?: unknown };
  recordedAt?: string;
  sourceCommit?: string;
  sourceDirty?: boolean;
  sourceDiffSha256?: string;
  binarySha256?: string;
  binaryVersion?: string;
  profileSha256?: string;
  recipeSha256?: string;
  scope?: string;
  limitations: string[];
  results: RecordedCase[];
}
