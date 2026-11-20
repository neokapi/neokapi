DO $migration$
DECLARE
    -- Project limits use the Unicode White_Space characters trimmed by Go's strings.TrimSpace.
    whitespace CONSTANT text := U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680' ||
        U&'\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000';
BEGIN
    IF to_regclass('kg_pilots') IS NULL OR to_regclass('kg_changesets') IS NULL
       OR to_regclass('workspaces') IS NULL THEN
        RETURN;
    END IF;

    LOCK TABLE kg_pilots, kg_changesets, workspaces IN SHARE ROW EXCLUSIVE MODE;
    DELETE FROM kg_pilots p
    WHERE p.project_id = '' OR p.project_id <> btrim(p.project_id, whitespace)
       OR strpos(p.project_id, ',') > 0 OR btrim(p.stream, whitespace) = ''
       OR NOT EXISTS (
           SELECT FROM kg_changesets cs JOIN workspaces w ON w.id = cs.workspace_id
           WHERE cs.workspace_id = p.workspace_id AND cs.id = p.changeset_id
             AND cs.status IN ('draft', 'in_review', 'approved')
       );

    IF NOT EXISTS (SELECT FROM tb_concepts WHERE starts_with(id, '__pilot__:c:'))
       AND NOT EXISTS (SELECT FROM tb_relations WHERE starts_with(id, '__pilot__:r:')) THEN
        RETURN;
    END IF;

    LOCK TABLE tb_concepts, tb_terms, tb_relations IN SHARE ROW EXCLUSIVE MODE;

    CREATE TEMP TABLE pilot_v11_concepts ON COMMIT DROP AS
    WITH bindings AS (
        SELECT w.slug AS workspace_id, p.project_id, p.stream,
               p.changeset_id || ':' || p.stream || ':' AS old_key,
               encode(convert_to(p.changeset_id, 'UTF8'), 'hex') || ':' ||
               encode(convert_to(p.project_id, 'UTF8'), 'hex') || ':' ||
               encode(convert_to(p.stream, 'UTF8'), 'hex') || ':' AS new_key
        FROM kg_pilots p JOIN workspaces w ON w.id = p.workspace_id
    )
    SELECT b.*, c.id AS old_id, '__pilot__:v2:c:' || b.new_key || ids.original_id AS new_id
    FROM bindings b
    JOIN tb_concepts c ON c.workspace_id = b.workspace_id AND c.stream = b.stream
        AND starts_with(c.id, '__pilot__:c:' || b.old_key)
    CROSS JOIN LATERAL (SELECT substr(c.id, length('__pilot__:c:' || b.old_key) + 1) AS original_id) ids
    LEFT JOIN tb_concepts original ON (original.workspace_id, original.id) = (c.workspace_id, ids.original_id)
    WHERE NOT EXISTS (
        SELECT FROM (VALUES (original.properties), (c.properties)) AS version(properties)
        CROSS JOIN LATERAL (
            SELECT array_remove(ARRAY(
                SELECT btrim(id, whitespace)
                FROM unnest(string_to_array(version.properties::jsonb ->> 'project_id', ',')) AS ids(id)
            ), '') AS project_ids
        ) scope
        WHERE cardinality(project_ids) > 0 AND NOT b.project_id = ANY(project_ids)
    );

    WITH inserted AS (
        INSERT INTO tb_concepts (
            workspace_id, id, stream, domain, definition, properties, source,
            do_not_translate, advisory, created_at, updated_at
        )
        SELECT c.workspace_id, m.new_id, c.stream, c.domain, c.definition,
               coalesce(nullif(c.properties::jsonb, 'null'), '{}') || jsonb_build_object('project_id', m.project_id),
               c.source, c.do_not_translate, c.advisory, c.created_at, c.updated_at
        FROM pilot_v11_concepts m JOIN tb_concepts c ON (c.workspace_id, c.id) = (m.workspace_id, m.old_id)
        ON CONFLICT (workspace_id, id) DO NOTHING
        RETURNING workspace_id, id
    )
    INSERT INTO tb_terms (
        workspace_id, concept_id, text, text_lower, text_normalized, locale, status, part_of_speech,
        gender, note, competitor_term, valid_from, valid_to, tags, forms
    )
    SELECT t.workspace_id, m.new_id, t.text, t.text_lower, t.text_normalized, t.locale, t.status,
           t.part_of_speech, t.gender, t.note, t.competitor_term, t.valid_from, t.valid_to, t.tags, t.forms
    FROM inserted i JOIN pilot_v11_concepts m ON (m.workspace_id, m.new_id) = (i.workspace_id, i.id)
    JOIN tb_terms t ON (t.workspace_id, t.concept_id) = (m.workspace_id, m.old_id);

    INSERT INTO tb_relations (
        workspace_id, id, source_id, target_id, relation, note, stream, valid_from, valid_to, tags, created_at
    )
    SELECT r.workspace_id,
           '__pilot__:v2:r:' || src.new_key || substr(r.id, length('__pilot__:r:' || src.old_key) + 1),
           src.new_id, dst.new_id, r.relation, r.note, r.stream, r.valid_from, r.valid_to, r.tags, r.created_at
    FROM tb_relations r
    JOIN pilot_v11_concepts src ON (src.workspace_id, src.old_id) = (r.workspace_id, r.source_id)
    JOIN pilot_v11_concepts dst ON (dst.workspace_id, dst.old_id, dst.new_key) = (r.workspace_id, r.target_id, src.new_key)
    WHERE r.stream = src.stream AND starts_with(r.id, '__pilot__:r:' || src.old_key)
    ON CONFLICT (workspace_id, id) DO NOTHING;

    INSERT INTO tb_revision (workspace_id, revision)
    SELECT workspace_id, replace(gen_random_uuid()::text, '-', '')
    FROM (
        SELECT workspace_id FROM tb_concepts WHERE starts_with(id, '__pilot__:c:')
        UNION
        SELECT workspace_id FROM tb_relations WHERE starts_with(id, '__pilot__:r:')
    ) affected
    ON CONFLICT (workspace_id) DO UPDATE SET revision = EXCLUDED.revision;

    DELETE FROM tb_relations WHERE starts_with(id, '__pilot__:r:');
    DELETE FROM tb_concepts WHERE starts_with(id, '__pilot__:c:');
END $migration$;
