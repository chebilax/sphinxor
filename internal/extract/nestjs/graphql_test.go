package nestjs

import (
	"testing"
)

// The GraphQL-first shape, reduced from notiz-dev/nestjs-prisma-starter:
// a resolver carrying the project's real authorization, beside a
// hello-world REST controller. The controller is what made the original
// silent — ADR 0019 §2's "recognized no endpoints" notice keys on zero,
// and two is not zero, so a single unrelated route suppressed the only
// signal there was.
const prismaStarterShape = `import { Resolver, Query, Mutation, ResolveField } from '@nestjs/graphql';
import { UseGuards } from '@nestjs/common';
import { GqlAuthGuard } from './gql-auth.guard';

@Resolver(() => User)
@UseGuards(GqlAuthGuard)
export class UsersResolver {
  @Query(() => User)
  async me(): Promise<User> { return null; }

  @Mutation(() => User)
  async updateUser(): Promise<User> { return null; }

  @ResolveField('posts')
  posts(): Post[] { return []; }
}
`

const helloRestController = `import { Controller, Get } from '@nestjs/common';

@Controller()
export class AppController {
  @Get()
  getHello(): string { return 'hello'; }
}
`

// TestGraphQLDetection is ADR 0021 §1's regression test.
//
// GraphQL is out of scope and stays out of scope; what must not happen
// is a project reporting a clean, confident run that describes a
// fraction of its API. On the real starter this reduction comes from,
// 16 operations — 10 of them guarded — produced "2 endpoint(s), 0
// finding(s)" and not one word about the rest.
func TestGraphQLDetection(t *testing.T) {
	t.Run("graphql-first project is detected", func(t *testing.T) {
		dir := writeTSProject(t, map[string]string{
			"users.resolver.ts": prismaStarterShape,
			"app.controller.ts": helloRestController,
		})
		m, _, err := Extract(dir)
		if err != nil {
			t.Fatalf("Extract: %v", err)
		}
		if !m.GraphQL.Present {
			t.Fatal("GraphQL.Present = false, want true — @Resolver is right there")
		}
		// The count is what makes the warning land: "this project uses
		// GraphQL" and "16 operations were not analyzed" read
		// differently.
		if m.GraphQL.Operations != 3 {
			t.Errorf("Operations = %d, want 3 (1 Query + 1 Mutation + 1 ResolveField)", m.GraphQL.Operations)
		}
		// Scope is unchanged: no resolver becomes an endpoint.
		if len(m.Endpoints) != 1 {
			t.Errorf("got %d endpoint(s), want 1 — only the REST route: %+v", len(m.Endpoints), m.Endpoints)
		}
	})

	t.Run("mixed project is detected too", func(t *testing.T) {
		// Reduced from CatsMiaow/nestjs-project-structure, and the more
		// dangerous case: its REST matrix is correct and complete, which
		// is exactly what makes the missing half easy to overlook. The
		// resolver's decorators are the same vocabulary the REST side
		// uses, and are still deliberately not extracted.
		dir := writeTSProject(t, map[string]string{
			"simple.resolver.ts": `import { Resolver, Query } from '@nestjs/graphql';
import { UseGuards } from '@nestjs/common';
import { Roles, RolesGuard } from '../common';

@Resolver(() => Simple)
export class SimpleResolver {
  @Query(() => Payload)
  @UseGuards(JwtAuthGuard, RolesGuard)
  @Roles('test')
  payload(): Payload { return null; }
}
`,
			"sample.controller.ts": `import { Controller, Get, Post } from '@nestjs/common';

@Controller('sample')
export class SampleController {
  @Get('hello')
  hello(): string { return 'x'; }

  @Post('body')
  body(): string { return 'x'; }
}
`,
		})
		m, _, err := Extract(dir)
		if err != nil {
			t.Fatalf("Extract: %v", err)
		}
		if !m.GraphQL.Present || m.GraphQL.Operations != 1 {
			t.Errorf("GraphQL = %+v, want Present with 1 operation", m.GraphQL)
		}
		if len(m.Endpoints) != 2 {
			t.Errorf("got %d endpoint(s), want the 2 REST routes: %+v", len(m.Endpoints), m.Endpoints)
		}
	})

	t.Run("a project without resolvers is not marked", func(t *testing.T) {
		// The noise floor. A caveat that fires everywhere is a caveat
		// nobody reads.
		dir := writeTSProject(t, map[string]string{"app.controller.ts": helloRestController})
		m, _, err := Extract(dir)
		if err != nil {
			t.Fatalf("Extract: %v", err)
		}
		if m.GraphQL.Present {
			t.Errorf("GraphQL = %+v, want absent on a REST-only project", m.GraphQL)
		}
	})
}
