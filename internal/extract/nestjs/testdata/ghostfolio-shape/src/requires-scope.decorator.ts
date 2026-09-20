import { applyDecorators, SetMetadata, UseGuards } from '@nestjs/common';
import { AuthGuard } from '@nestjs/passport';

import { HasPermissionGuard } from './has-permission.guard';
import { ImpersonationGuard } from './impersonation.guard';
import { ScopeGuard } from './scope.guard';
import { Scope } from './scopes';

export const REQUIRES_SCOPE_KEY = 'requires_scope';

// Declares the scopes a route requires, and applies the guards that resolve
// and evaluate them. The scopes travel as metadata; the guards enforce.
export function RequiresScope(...requiredScopes: Scope[]) {
  return applyDecorators(
    SetMetadata(REQUIRES_SCOPE_KEY, requiredScopes),
    UseGuards(AuthGuard('jwt'), HasPermissionGuard, ImpersonationGuard, ScopeGuard),
  );
}
