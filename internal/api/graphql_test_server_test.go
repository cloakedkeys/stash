package api

import (
	"github.com/99designs/gqlgen/graphql/handler"
	gqlTransport "github.com/99designs/gqlgen/graphql/handler/transport"
)

// newTestGraphQLServer serves the real schema and directives over POST, as the tests send.
func newTestGraphQLServer(r *Resolver) *handler.Server {
	srv := handler.New(NewExecutableSchema(Config{Resolvers: r, Directives: DirectiveRoot{HasRole: HasRoleDirective, IsUserOwner: IsUserOwnerDirective}}))
	srv.AddTransport(gqlTransport.POST{})
	return srv
}
