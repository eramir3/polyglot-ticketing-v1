package ticket

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (repository *PostgresRepository) Create(ctx context.Context, input CreateInput) (Ticket, error) {
	var created Ticket
	err := repository.pool.QueryRow(
		ctx,
		`INSERT INTO tickets (title, price, user_id)
		 VALUES ($1, $2, $3)
		 RETURNING id, title, price, user_id`,
		input.Title,
		input.Price,
		input.UserID,
	).Scan(&created.ID, &created.Title, &created.Price, &created.UserID)

	return created, err
}

func (repository *PostgresRepository) List(ctx context.Context) ([]Ticket, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT id, title, price, user_id
		 FROM tickets
		 ORDER BY title ASC, id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tickets := make([]Ticket, 0)
	for rows.Next() {
		var listed Ticket
		if err := rows.Scan(&listed.ID, &listed.Title, &listed.Price, &listed.UserID); err != nil {
			return nil, err
		}

		tickets = append(tickets, listed)
	}

	return tickets, rows.Err()
}
