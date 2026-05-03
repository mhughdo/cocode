export function requireWorkspaceMember(request, response, next) {
  if (!request.user) {
    response.status(401).end();
    return;
  }
  next();
}

export function requireWorkspaceAdmin(request, response, next) {
  if (request.user?.role !== "admin") {
    response.status(403).end();
    return;
  }
  next();
}
