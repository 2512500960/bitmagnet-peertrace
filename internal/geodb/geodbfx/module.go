package geodbfx

import (
	"github.com/bitmagnet-io/bitmagnet/internal/geodb"
	"go.uber.org/fx"
)

func New() fx.Option {
	return fx.Module(
		"geodb",
		fx.Provide(
			geodb.NewGeoDB,
		),
	)
}
