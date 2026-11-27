-- Client-grade classification on findings (E1): a CVSS v4.0 vector with the
-- score the server computed from it (never a caller-supplied score), an
-- optional EPSS estimate, remediation guidance and framework tags (CWE, OWASP
-- ASVS/WSTG, MITRE ATT&CK) as a JSON array of {framework, id}. The Go type is
-- vulnmeta.Classification; db/finding_classification.go reads and writes it.
-- Every column defaults to "not classified", so existing rows need no backfill.
ALTER TABLE findings ADD COLUMN IF NOT EXISTS cvss_vector      TEXT NOT NULL DEFAULT '';
ALTER TABLE findings ADD COLUMN IF NOT EXISTS cvss_score       NUMERIC(3,1)
    CHECK (cvss_score IS NULL OR (cvss_score >= 0 AND cvss_score <= 10));
ALTER TABLE findings ADD COLUMN IF NOT EXISTS epss_probability DOUBLE PRECISION
    CHECK (epss_probability IS NULL OR (epss_probability >= 0 AND epss_probability <= 1));
ALTER TABLE findings ADD COLUMN IF NOT EXISTS epss_percentile  DOUBLE PRECISION
    CHECK (epss_percentile IS NULL OR (epss_percentile >= 0 AND epss_percentile <= 1));
ALTER TABLE findings ADD COLUMN IF NOT EXISTS epss_date        DATE;
ALTER TABLE findings ADD COLUMN IF NOT EXISTS epss_cve         TEXT NOT NULL DEFAULT '';
ALTER TABLE findings ADD COLUMN IF NOT EXISTS remediation      TEXT NOT NULL DEFAULT '';
ALTER TABLE findings ADD COLUMN IF NOT EXISTS framework_tags   JSONB NOT NULL DEFAULT '[]';
-- "Which findings map to CWE-89 / WSTG-INPV-05" is a containment lookup.
CREATE INDEX IF NOT EXISTS idx_findings_framework_tags ON findings USING GIN(framework_tags jsonb_path_ops);
