-- Fictional West Village venues for local development. Names end in "(seed)" so they
-- can't be mistaken for real data. Re-running is a no-op.
INSERT INTO venues (id, name, address, location, timezone, website, attribution) VALUES
  ('5eed0000-0000-0000-0000-000000000001', 'Grove Street Tap (seed)', '1 Grove St, New York, NY 10014',
   ST_SetSRID(ST_MakePoint(-74.0030, 40.7334), 4326)::geography, 'America/New_York', 'https://example.com/grove', '{}'),
  ('5eed0000-0000-0000-0000-000000000002', 'Bleecker Night Owl (seed)', '2 Bleecker St, New York, NY 10014',
   ST_SetSRID(ST_MakePoint(-74.0045, 40.7325), 4326)::geography, 'America/New_York', NULL, '{}'),
  ('5eed0000-0000-0000-0000-000000000003', 'Hudson Oyster Room (seed)', '3 Hudson St, New York, NY 10014',
   ST_SetSRID(ST_MakePoint(-74.0072, 40.7340), 4326)::geography, 'America/New_York', 'https://example.com/oyster', '{}'),
  ('5eed0000-0000-0000-0000-000000000004', 'Christopher Corner (seed)', '4 Christopher St, New York, NY 10014',
   ST_SetSRID(ST_MakePoint(-74.0010, 40.7338), 4326)::geography, 'America/New_York', NULL, '{}')
ON CONFLICT (id) DO NOTHING;

INSERT INTO happy_hours (id, venue_id, day_of_week, start_time, end_time, all_day, deals, notes, confidence, verified, source_kind, observed_at)
SELECT gen_random_uuid(), v.venue_id::uuid, d, v.start_t::time, v.end_t::time, v.all_day, v.deals::jsonb, v.notes, v.confidence, true, 'field_photo', '2026-10-03'
FROM (VALUES
  -- Mon–Fri 16:00–19:00
  ('5eed0000-0000-0000-0000-000000000001', 1, 5, '16:00', '19:00', false, '[{"item":"Draft beer","price":5},{"item":"Well drinks","price":7}]', NULL, 0.9),
  -- Late-night window crossing midnight, Thu–Sat 22:00–01:00
  ('5eed0000-0000-0000-0000-000000000002', 4, 6, '22:00', '01:00', false, '[{"item":"Shot and a beer","price":8}]', 'Bar area only', 0.85),
  -- Oysters Mon–Fri 17:00–18:30
  ('5eed0000-0000-0000-0000-000000000003', 1, 5, '17:00', '18:30', false, '[{"item":"Oysters","price":1.5,"note":"each"}]', NULL, 0.95),
  -- All day Sunday
  ('5eed0000-0000-0000-0000-000000000004', 0, 0, NULL, NULL, true, '[{"item":"Mimosas","price":6}]', NULL, 0.8)
) AS v(venue_id, first_day, last_day, start_t, end_t, all_day, deals, notes, confidence)
CROSS JOIN LATERAL generate_series(v.first_day, v.last_day) AS d
WHERE NOT EXISTS (SELECT 1 FROM happy_hours h WHERE h.venue_id = v.venue_id::uuid);
