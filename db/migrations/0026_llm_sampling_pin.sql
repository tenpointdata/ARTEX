-- E8 follow-up: optional per-profile sampling pin for reproducible A/B runs.
-- NULL = not pinned (the historical behaviour: nothing sent, provider default applies).
--   sampling_seed        sent as the request "seed" field, only for formats whose wire API
--                        accepts it (openai Chat Completions, ollama); the API refuses it elsewhere.
--   sampling_temperature used as the request temperature when the caller sets none; a code-owned
--                        temperature (for example the interception assessment's) is left alone.
ALTER TABLE llm_profiles ADD COLUMN IF NOT EXISTS sampling_seed        BIGINT;
ALTER TABLE llm_profiles ADD COLUMN IF NOT EXISTS sampling_temperature DOUBLE PRECISION;
ALTER TABLE llm_profiles DROP CONSTRAINT IF EXISTS llm_profiles_sampling_temperature_check;
ALTER TABLE llm_profiles ADD  CONSTRAINT llm_profiles_sampling_temperature_check
    CHECK (sampling_temperature IS NULL OR (sampling_temperature >= 0 AND sampling_temperature <= 2));
