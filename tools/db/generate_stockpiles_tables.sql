-- Creates the tables behind the Stockpiles page, in the stockpiles schema.
--   structures       player structures a signed-in character could read,
--                    refreshed from ESI at sign-in; stockpiles are tied to one
--   stockpiles       named groups of items wanted at a structure, shared by
--                    everyone who signs in
--   stockpile_items  what each stockpile wants: a quantity of an item, or
--                    runs of a blueprint (kind = 'blueprint_runs')
-- Run it against the meadow_works database, e.g.
--   psql -d meadow_works -f tools/db/generate_stockpiles_tables.sql
-- Safe to run more than once.
begin;
create schema if not exists stockpiles;
set search_path to stockpiles;

create table if not exists structures (
    structure_id bigint primary key,
    name text not null,
    solar_system text not null default '',
    region text not null default '',
    type text not null default '',
    owner text not null default '',
    updated_at timestamptz not null default now()
);

create table if not exists stockpiles (
    stockpile_id bigserial primary key,
    name text not null,
    structure_id bigint not null references structures (structure_id),
    created_by_character_id bigint not null,
    created_by_name text not null,
    created_at timestamptz not null default now()
);

create table if not exists stockpile_items (
    stockpile_item_id bigserial primary key,
    stockpile_id bigint not null references stockpiles (stockpile_id) on delete cascade,
    type_id int not null,
    name text not null,
    kind text not null check (kind in ('item', 'blueprint_runs')),
    quantity bigint not null check (quantity > 0),
    unique (stockpile_id, type_id, kind)
);
commit;
