package spring

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func urlLayerOf(t *testing.T, files map[string]string) (present, analyzed bool, reason string) {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m, _, err := Extract(dir)
	if err != nil {
		t.Fatal(err)
	}
	return m.URLLayer.Present, m.URLLayer.Analyzed, m.URLLayer.Reason
}

const aController = `package app;
import org.springframework.web.bind.annotation.*;

@RestController
public class C {
    @PostMapping("/a")
    public void a() { }
}
`

// TestURLLayer_LegacyAdapterDetected is ADR 0027 §1/§2 for
// WebSecurityConfigurerAdapter — Spring Security's own pre-5.7
// configuration base class, in scope per ADR 0011 and announced by
// nothing before this. zalando/nakadi is the corpus instance.
func TestURLLayer_LegacyAdapterDetected(t *testing.T) {
	present, analyzed, reason := urlLayerOf(t, map[string]string{
		"C.java": aController,
		"SecurityConfiguration.java": `package app;
import org.springframework.security.config.annotation.web.configuration.WebSecurityConfigurerAdapter;

public class SecurityConfiguration extends WebSecurityConfigurerAdapter {
    protected void configure(HttpSecurity http) throws Exception {
        http.authorizeRequests().antMatchers("/a").access("#oauth2.hasScope('x')");
    }
}
`,
	})
	if !present || analyzed {
		t.Fatalf("a WebSecurityConfigurerAdapter must be present-but-unanalyzed, got present=%v analyzed=%v", present, analyzed)
	}
	if !strings.Contains(reason, "WebSecurityConfigurerAdapter") {
		t.Errorf("reason should name the form, got %q", reason)
	}
}

// TestURLLayer_ShiroDetected is ADR 0027 §1/§2 for Shiro. Nothing is
// parsed: recording that an authorization layer exists is not
// interpreting it, which is the narrower half of ADR 0023's argument.
func TestURLLayer_ShiroDetected(t *testing.T) {
	present, analyzed, reason := urlLayerOf(t, map[string]string{
		"C.java": aController,
		"ShiroConfig.java": `package app;
import org.apache.shiro.spring.web.ShiroFilterFactoryBean;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

@Configuration
public class ShiroConfig {
    @Bean
    public ShiroFilterFactoryBean shiroFilter(SecurityManager securityManager) {
        return new ShiroFilterFactoryBean();
    }
}
`,
	})
	if !present || analyzed {
		t.Fatalf("a ShiroFilterFactoryBean must be present-but-unanalyzed, got present=%v analyzed=%v", present, analyzed)
	}
	if !strings.Contains(reason, "Shiro") {
		t.Errorf("the reason must name Shiro — otherwise a reader goes looking for a SecurityFilterChain they do not have; got %q", reason)
	}
}

// TestURLLayer_MentionIsNotDeclaration pins the near-miss that shaped
// ADR 0027 §1.
//
// A first count by grep suggested apache/shenyu had an unannounced
// reactive SecurityWebFilterChain. It does not: the type appears only in
// an import and a @ConditionalOnClass, with no @Bean returning one. A
// token-presence check would have invented a URL layer out of an import,
// which is the same "spelling is not identity" fault ADR 0022 and
// ADR 0023 each corrected in their own domain.
func TestURLLayer_MentionIsNotDeclaration(t *testing.T) {
	present, _, reason := urlLayerOf(t, map[string]string{
		"C.java": aController,
		"OAuth2PluginConfiguration.java": `package app;
import org.springframework.security.web.server.SecurityWebFilterChain;
import org.apache.shiro.spring.web.ShiroFilterFactoryBean;

@Configuration
@ConditionalOnClass({ SecurityWebFilterChain.class, ShiroFilterFactoryBean.class })
public class OAuth2PluginConfiguration {
    @Bean
    public Object somethingElse() { return null; }
}
`,
	})
	if present {
		t.Errorf("an import and a @ConditionalOnClass are mentions, not declarations; got a URL layer with reason %q", reason)
	}
}

// TestURLLayer_TwoUnreadableLayersNameBoth is ADR 0027 §3, and the case
// that forced it: JeecgBoot declares both a reactive SecurityWebFilterChain
// and a ShiroFilterFactoryBean. URLLayerStatus holds one Reason, so the
// previous switch would have let one hide the other.
func TestURLLayer_TwoUnreadableLayersNameBoth(t *testing.T) {
	_, analyzed, reason := urlLayerOf(t, map[string]string{
		"C.java": aController,
		"Both.java": `package app;
import org.apache.shiro.spring.web.ShiroFilterFactoryBean;
import org.springframework.security.web.server.SecurityWebFilterChain;
import org.springframework.context.annotation.Bean;

@Configuration
public class Both {
    @Bean
    public SecurityWebFilterChain reactiveChain(ServerHttpSecurity http) { return null; }

    @Bean
    public ShiroFilterFactoryBean shiroFilter(SecurityManager sm) { return new ShiroFilterFactoryBean(); }
}
`,
	})
	if analyzed {
		t.Fatal("two unreadable layers must leave the URL layer unanalyzed")
	}
	if !strings.Contains(reason, "SecurityWebFilterChain") || !strings.Contains(reason, "Shiro") {
		t.Errorf("both layers must be named, neither hiding the other; got %q", reason)
	}
}

// TestURLLayer_ParsedChainStaysQuiet is ADR 0027 §4's noise floor at unit
// level: a single SecurityFilterChain whose rules parse is Present AND
// Analyzed, and must not acquire a caveat. RuoYi-Vue is the corpus
// instance; Pharmacy is the fixture one.
func TestURLLayer_ParsedChainStaysQuiet(t *testing.T) {
	present, analyzed, reason := urlLayerOf(t, map[string]string{
		"C.java": aController,
		"SecurityConfig.java": `package app;
import org.springframework.context.annotation.Bean;
import org.springframework.security.web.SecurityFilterChain;

@Configuration
public class SecurityConfig {
    @Bean
    public SecurityFilterChain filterChain(HttpSecurity http) throws Exception {
        http.authorizeHttpRequests(requests -> requests
            .requestMatchers("/a").hasRole("ADMIN")
            .anyRequest().authenticated());
        return http.build();
    }
}
`,
	})
	if !present || !analyzed {
		t.Errorf("a parseable single chain must be analyzed and quiet, got present=%v analyzed=%v reason=%q", present, analyzed, reason)
	}
}
