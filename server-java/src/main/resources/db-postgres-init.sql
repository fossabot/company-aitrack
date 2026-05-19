-- Run after initial schema creation on ParadeDB
-- Creates BM25 full-text and HNSW vector indexes
-- NOTE: Table name is edit_records (mapped from EditRecordEntity)
-- This file is a reference for the DBA — not auto-run by Spring Boot.

-- BM25 full-text index on diff_hunk and prompt_summary
CREATE INDEX IF NOT EXISTS edit_records_bm25 ON edit_records
USING bm25 (id, diff_hunk, prompt_summary)
WITH (key_field = 'id');

-- HNSW vector index (placeholder; embedding col is null until Phase DB-2)
CREATE INDEX IF NOT EXISTS edit_records_hnsw ON edit_records
USING hnsw (embedding vector_cosine_ops)
WHERE embedding IS NOT NULL;
