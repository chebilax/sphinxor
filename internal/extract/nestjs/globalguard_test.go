package nestjs

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDetectGlobalGuards covers ADR 0020 §4's NestJS half: both ways a
// guard gets registered for every route without appearing at any
// endpoint. Verified against the shape in nestjs/nest's own 19-auth-jwt
// sample, where every endpoint reports no guard because the guard is
// global and @Public() opts out.
//
// Detection is deliberately shallow — it establishes that the default is
// inverted, not what the guard requires. What it must not do is stay
// silent, which is what made this a blind spot rather than a caveat.
func TestDetectGlobalGuards(t *testing.T) {
	cases := []struct {
		name           string
		source         string
		wantRegistered bool
		wantMechanism  string
	}{
		{
			name: "APP_GUARD provider",
			source: `import { APP_GUARD } from '@nestjs/core';
@Module({ providers: [{ provide: APP_GUARD, useClass: JwtAuthGuard }] })
export class AppModule {}
`,
			wantRegistered: true,
			wantMechanism:  "an APP_GUARD provider",
		},
		{
			name: "useGlobalGuards at bootstrap",
			source: `async function bootstrap() {
  const app = await NestFactory.create(AppModule);
  app.useGlobalGuards(new JwtAuthGuard());
}
`,
			wantRegistered: true,
			wantMechanism:  "an app.useGlobalGuards() call",
		},
		{
			// The ordinary case, which must stay silent: a per-endpoint
			// guard says nothing about the project-wide default.
			name: "local UseGuards only",
			source: `@Controller('cats')
export class CatsController {
  @UseGuards(RolesGuard)
  @Post()
  create() {}
}
`,
			wantRegistered: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "src", "app.ts")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.source), 0o644); err != nil {
				t.Fatal(err)
			}

			m, _, err := Extract(dir)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			if m.GlobalGuards.Registered != tc.wantRegistered {
				t.Fatalf("Registered = %v, want %v", m.GlobalGuards.Registered, tc.wantRegistered)
			}
			if tc.wantRegistered && m.GlobalGuards.Mechanism != tc.wantMechanism {
				t.Errorf("Mechanism = %q, want %q — the warning names it, so it has to be right",
					m.GlobalGuards.Mechanism, tc.wantMechanism)
			}
		})
	}
}
