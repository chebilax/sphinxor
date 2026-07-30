package nestjs

import "testing"

// TestExtractRoleDeclarations_FiltersUnrelatedEnums is a regression test
// for a false-positive found empirically against brocoders/nestjs-boilerplate:
// extracting every enum in a project as a candidate role produced noise
// from enums with nothing to do with authorization (environment config,
// file storage driver, ...). Only enums a @Roles() call actually names
// should be treated as role declarations.
func TestExtractRoleDeclarations_FiltersUnrelatedEnums(t *testing.T) {
	src := `
enum Environment {
  Development = 'development',
  Production = 'production',
}

enum RoleEnum {
  admin = 'admin',
  user = 'user',
}

@Controller('users')
export class UsersController {
  @UseGuards(RolesGuard)
  @Roles(RoleEnum.admin)
  @Post()
  create() {}
}
`
	root, source := parseTS(t, src)
	used := collectRoleEnumNames(root, source)

	if used["Environment"] {
		t.Errorf("Environment should not be considered a role enum: no @Roles() call names it")
	}
	if !used["RoleEnum"] {
		t.Errorf("RoleEnum should be considered a role enum: @Roles(RoleEnum.admin) names it")
	}

	b := newBuilder()
	decls := extractRoleDeclarations(root, source, "f.ts", b.nextID("role"), used)

	names := make(map[string]bool, len(decls))
	for _, d := range decls {
		names[d.Name] = true
	}
	if names["Environment.Development"] || names["Environment.Production"] {
		t.Errorf("Environment members should be filtered out, got declarations: %v", decls)
	}
	if !names["RoleEnum.admin"] || !names["RoleEnum.user"] {
		t.Errorf("expected both RoleEnum members declared, got: %v", decls)
	}
}

// TestExtractRoleDeclarations_StringKeyedEnum covers the real-world
// pattern seen in brocoders/nestjs-boilerplate, where enum member names
// are themselves string literals rather than bare identifiers
// (`'admin' = 1` rather than `admin = 'admin'`).
func TestExtractRoleDeclarations_StringKeyedEnum(t *testing.T) {
	src := `
enum RoleEnum {
  'admin' = 1,
  'user' = 2,
}

@Controller('users')
export class UsersController {
  @Roles(RoleEnum.admin)
  @Post()
  create() {}
}
`
	root, source := parseTS(t, src)
	used := collectRoleEnumNames(root, source)
	b := newBuilder()
	decls := extractRoleDeclarations(root, source, "f.ts", b.nextID("role"), used)

	names := make(map[string]bool, len(decls))
	for _, d := range decls {
		names[d.Name] = true
	}
	if !names["RoleEnum.admin"] || !names["RoleEnum.user"] {
		t.Errorf("expected RoleEnum.admin and RoleEnum.user from string-keyed enum, got: %v", decls)
	}
}
