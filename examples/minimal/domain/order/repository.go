package order

import "github.com/slam0504/go-ddd-core/domain"

// Repository narrows domain.Repository to the Order aggregate.
type Repository = domain.Repository[*Order, ID]
