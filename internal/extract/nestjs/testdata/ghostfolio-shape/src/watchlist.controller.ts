import { Body, Controller, Delete, Get, Param, Post } from '@nestjs/common';

import { RequiresScope } from './requires-scope.decorator';
import { scopes } from './scopes';

@Controller('watchlist')
export class WatchlistController {
  @Post()
  @RequiresScope(scopes.watchlistCreate)
  public async createWatchlistItem(@Body() body: unknown) {
    return body;
  }

  @Delete(':dataSource/:symbol')
  @RequiresScope(scopes.watchlistDelete)
  public async deleteWatchlistItem(@Param('symbol') symbol: string) {
    return symbol;
  }

  @Get()
  @RequiresScope(scopes.watchlistRead)
  public async getWatchlistItems() {
    return [];
  }

  // Deliberately undecorated: a genuine true positive for
  // mutating-endpoint-without-access-control, so the fixture proves the
  // rule still fires rather than only that it stopped firing.
  @Post('import')
  public async importWatchlist(@Body() body: unknown) {
    return body;
  }
}
