# Provenance

Vendored, unmodified, from https://github.com/alibaba/nacos,
commit `b4fec02964c8606ad6b2ea954bd90d96023d4d00`, Apache License 2.0.
The upstream `LICENSE` is copied alongside, as section 4(a) of that license
requires, and so is the upstream `NOTICE`, as section 4(d) requires.

Used per docs/testing.md and docs/decisions/0039-route-path-constants.md §8:
the **duplicate simple name** fixture for route paths.

Files, and why each was picked:

- `ai/.../controller/A2aAdminController.java` —
  `@RequestMapping(Constants.A2A.ADMIN_PATH)`: a constant in a nested class, whose
  outer class is imported by name (`import com.alibaba.nacos.ai.constant.Constants`).
- `ai/.../constant/Constants.java` — that class; `A2A.ADMIN_PATH` is
  `/v3/admin/ai/a2a`.
- `lock/.../constant/Constants.java` — a second nacos class named `Constants`, in
  another package. nacos declares nine; this is the smallest (28 lines), chosen
  from among them, unmodified. With both present, a lookup by simple name is
  ambiguous and must refuse — which is what ADR 0038's index did, and why 209 of
  nacos's routes stayed unreadable. Resolution through the file's own import
  settles it without a guess.

Three files from one repository, against ADR 0005 Amendment 1's trigger of about
twenty.
