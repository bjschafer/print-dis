-- +goose Up
-- A request lists any number of colors (docs/design.md section 5); the single
-- filament snapshot and color note become the first entry of that list.
alter table requests add column colors jsonb not null default '[]';
update requests set colors = jsonb_build_array(jsonb_strip_nulls(jsonb_build_object(
    'filament_id', spoolman_filament_id,
    'label',       nullif(filament_label, ''),
    'material',    case when spoolman_filament_id is not null then nullif(material, '') end,
    'hexes',       case when filament_color_hex is not null then jsonb_build_array(filament_color_hex) end,
    'note',        nullif(color_note, ''))))
  where spoolman_filament_id is not null or filament_label <> '' or filament_color_hex is not null or color_note <> '';
-- A note that is just a palette color's name ("black", from the sheet
-- import) becomes that palette color, so it shows a swatch. The palette is
-- core.RoughColors as of this migration.
update requests r set colors = jsonb_build_array(jsonb_build_object(
    'rough', k.key, 'label', p.name, 'hexes', jsonb_build_array(p.hex)))
  from (values
    ('red', 'Red', '#d32f2f'), ('orange', 'Orange', '#f57c00'), ('yellow', 'Yellow', '#fbc02d'),
    ('green', 'Green', '#388e3c'), ('teal', 'Teal', '#00897b'), ('blue', 'Blue', '#1e66d0'),
    ('purple', 'Purple', '#7b1fa2'), ('pink', 'Pink', '#ec6fa8'), ('brown', 'Brown', '#795548'),
    ('black', 'Black', '#161616'), ('grey', 'Gray', '#8a8f98'), ('gray', 'Gray', '#8a8f98'),
    ('white', 'White', '#f4f4f2'), ('silver', 'Silver', '#c0c4ca'), ('gold', 'Gold', '#c9a227'),
    ('clear', 'Clear', '#dcebf0')) as p(word, name, hex),
    lateral (select case p.word when 'gray' then 'grey' else p.word end as key) k
  where r.spoolman_filament_id is null and r.filament_label = '' and r.filament_color_hex is null
    and lower(trim(r.color_note)) = p.word;
-- "any" was never a color.
update requests set colors = '[]' where spoolman_filament_id is null and lower(trim(color_note)) = 'any';
alter table requests
  drop column spoolman_filament_id,
  drop column filament_label,
  drop column filament_color_hex,
  drop column color_note;

-- +goose Down
alter table requests
  add column spoolman_filament_id integer,
  add column filament_label       text not null default '',
  add column filament_color_hex   text,
  add column color_note           text not null default '';
update requests set
    spoolman_filament_id = (colors->0->>'filament_id')::integer,
    filament_label       = coalesce(colors->0->>'label', ''),
    filament_color_hex   = colors->0->'hexes'->>0,
    color_note           = coalesce(colors->0->>'note', '')
  where jsonb_array_length(colors) > 0;
alter table requests drop column colors;
