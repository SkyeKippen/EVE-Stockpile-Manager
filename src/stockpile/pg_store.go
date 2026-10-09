package stockpile

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgStore keeps stockpiles in the stockpiles schema; create its tables with
// tools/db/generate_stockpiles_tables.sql.
type PgStore struct {
	Pool *pgxpool.Pool
}

func NewPgStore(ctx context.Context, databaseUrl string) (*PgStore, error) {
	pool, err := pgxpool.New(ctx, databaseUrl)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &PgStore{Pool: pool}, nil
}

func (p *PgStore) ListStockpiles(ctx context.Context) ([]Stockpile, error) {
	rows, err := p.Pool.Query(ctx, `
		SELECT s.stockpile_id, s.name, s.created_by_name,
		       t.structure_id, t.name, t.solar_system, t.region, t.type, t.owner
		FROM stockpiles.stockpiles s
		JOIN stockpiles.structures t ON t.structure_id = s.structure_id`)
	if err != nil {
		return nil, err
	}
	stockpiles, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Stockpile, error) {
		var s Stockpile
		err := row.Scan(&s.StockpileId, &s.Name, &s.CreatedByName,
			&s.Structure.StructureId, &s.Structure.Name, &s.Structure.System, &s.Structure.Region, &s.Structure.Type, &s.Structure.Owner)
		return s, err
	})
	if err != nil {
		return nil, err
	}

	rows, err = p.Pool.Query(ctx, `
		SELECT stockpile_id, stockpile_item_id, type_id, name, kind, quantity
		FROM stockpiles.stockpile_items`)
	if err != nil {
		return nil, err
	}
	itemsByStockpile := map[int64][]Item{}
	var stockpileId int64
	var item Item
	_, err = pgx.ForEachRow(rows, []any{&stockpileId, &item.StockpileItemId, &item.TypeId, &item.Name, &item.Kind, &item.Quantity}, func() error {
		itemsByStockpile[stockpileId] = append(itemsByStockpile[stockpileId], item)
		return nil
	})
	if err != nil {
		return nil, err
	}

	for i := range stockpiles {
		items := itemsByStockpile[stockpiles[i].StockpileId]
		slices.SortFunc(items, func(a, b Item) int { return compare(a.Name, b.Name) })
		stockpiles[i].Items = items
	}
	slices.SortFunc(stockpiles, func(a, b Stockpile) int { return compare(a.Name, b.Name) })
	return stockpiles, nil
}

func (p *PgStore) CreateStockpile(ctx context.Context, name string, structureId int64, createdById int64, createdByName string) (int64, error) {
	var id int64
	err := p.Pool.QueryRow(ctx, `
		INSERT INTO stockpiles.stockpiles (name, structure_id, created_by_character_id, created_by_name)
		VALUES ($1, $2, $3, $4)
		RETURNING stockpile_id`, name, structureId, createdById, createdByName).Scan(&id)
	if isForeignKeyViolation(err) {
		return 0, ErrNotFound
	}
	return id, err
}

func (p *PgStore) DeleteStockpile(ctx context.Context, stockpileId int64) error {
	_, err := p.Pool.Exec(ctx, `DELETE FROM stockpiles.stockpiles WHERE stockpile_id = $1`, stockpileId)
	return err
}

func (p *PgStore) SetItem(ctx context.Context, stockpileId int64, item Item) error {
	_, err := p.Pool.Exec(ctx, `
		INSERT INTO stockpiles.stockpile_items (stockpile_id, type_id, name, kind, quantity)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (stockpile_id, type_id, kind) DO UPDATE SET quantity = excluded.quantity, name = excluded.name`,
		stockpileId, item.TypeId, item.Name, item.Kind, item.Quantity)
	if isForeignKeyViolation(err) {
		return ErrNotFound
	}
	return err
}

func (p *PgStore) RemoveItem(ctx context.Context, stockpileId, stockpileItemId int64) error {
	_, err := p.Pool.Exec(ctx, `
		DELETE FROM stockpiles.stockpile_items WHERE stockpile_id = $1 AND stockpile_item_id = $2`,
		stockpileId, stockpileItemId)
	return err
}

func (p *PgStore) SaveStructures(ctx context.Context, structures []Structure) error {
	batch := &pgx.Batch{}
	for _, s := range structures {
		batch.Queue(`
			INSERT INTO stockpiles.structures (structure_id, name, solar_system, region, type, owner, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, now())
			ON CONFLICT (structure_id) DO UPDATE SET
				name = excluded.name, solar_system = excluded.solar_system, region = excluded.region,
				type = excluded.type, owner = excluded.owner, updated_at = now()`,
			s.StructureId, s.Name, s.System, s.Region, s.Type, s.Owner)
	}
	return p.Pool.SendBatch(ctx, batch).Close()
}

func (p *PgStore) SearchStructures(ctx context.Context, query string, limit int) ([]Structure, error) {
	rows, err := p.Pool.Query(ctx, `
		SELECT structure_id, name, solar_system, region, type, owner
		FROM stockpiles.structures
		WHERE name ILIKE '%' || $1 || '%'`, escapeLike(strings.TrimSpace(query)))
	if err != nil {
		return nil, err
	}
	structures, err := pgx.CollectRows(rows, scanStructure)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(structures, func(a, b Structure) int { return compare(a.Name, b.Name) })
	return structures[:min(limit, len(structures))], nil
}

func (p *PgStore) GetStructure(ctx context.Context, structureId int64) (Structure, error) {
	rows, err := p.Pool.Query(ctx, `
		SELECT structure_id, name, solar_system, region, type, owner
		FROM stockpiles.structures WHERE structure_id = $1`, structureId)
	if err != nil {
		return Structure{}, err
	}
	s, err := pgx.CollectExactlyOneRow(rows, scanStructure)
	if errors.Is(err, pgx.ErrNoRows) {
		return Structure{}, ErrNotFound
	}
	return s, err
}

func scanStructure(row pgx.CollectableRow) (Structure, error) {
	var s Structure
	err := row.Scan(&s.StructureId, &s.Name, &s.System, &s.Region, &s.Type, &s.Owner)
	return s, err
}

// escapeLike stops % and _ in a search from acting as wildcards.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func isForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23503"
}
