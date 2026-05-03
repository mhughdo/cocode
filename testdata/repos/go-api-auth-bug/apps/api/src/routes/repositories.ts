import {
  requireWorkspaceAdmin,
  requireWorkspaceMember,
} from "../middleware/auth";
import { repositoryService } from "../services/repository-service";
import { Router } from "../types";

const router = new Router();

router.get(
  "/repositories/:id",
  requireWorkspaceMember,
  async (request, response) => {
    const repository = await repositoryService.getRepository(request.params.id);
    response.json(repository);
  },
);

router.patch(
  "/repositories/:id/settings",
  requireWorkspaceMember,
  updateRepositorySettings,
);

export async function updateRepositorySettings(request, response) {
  const settings = await repositoryService.updateSettings(
    request.params.id,
    request.body,
  );
  response.json(settings);
}

export function registerAdminRoutes(adminRouter) {
  adminRouter.delete(
    "/repositories/:id",
    requireWorkspaceAdmin,
    async (request, response) => {
      await repositoryService.archiveRepository(request.params.id);
      response.status(204).end();
    },
  );
}

export { router };
