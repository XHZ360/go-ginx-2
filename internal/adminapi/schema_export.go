package adminapi

import "github.com/graphql-go/graphql"

// GraphQLSchema builds the admin GraphQL schema without binding a listener.
func GraphQLSchema() (graphql.Schema, error) {
	return (&Server{}).buildSchema()
}
