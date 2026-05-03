import { router } from "./repositories";

test("members can read repositories", async () => {
  const response = await router.inject({
    method: "GET",
    url: "/repositories/repo_1",
    user: { id: "user_1", role: "member" },
  });

  expect(response.statusCode).toBe(200);
});
