-- Creates the stockpiles schema and its items table: how many of each item
-- type is stockpiled at each location. Run it against the meadow_works
-- database, e.g. psql -d meadow_works -f tools/db/generate_items_table.sql
-- Safe to run more than once.
create schema if not exists stockpiles;
set search_path to stockpiles;

create table if not exists items (
    item_id bigserial primary key,
    type_id int not null,
    name text not null,
    quantity bigint not null default 0,
    location text not null,
    unique (type_id, location)
);
