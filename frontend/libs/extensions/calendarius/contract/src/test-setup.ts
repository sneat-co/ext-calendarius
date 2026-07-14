import '@analogjs/vitest-angular/setup-zone';
import { setupTestBed } from '@analogjs/vitest-angular/setup-testbed';

// Keep the contract tests independent of unpublished workspace-only testing
// entry points from sneat-libs.
setupTestBed({ zoneless: false });
