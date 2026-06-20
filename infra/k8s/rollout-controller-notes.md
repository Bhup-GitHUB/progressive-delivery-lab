# Rollout Controller Notes

These Kubernetes files are examples for explaining the pattern. They are not a production rollout system.

In a real cluster, traffic splitting could be handled by an ingress controller, service mesh, or progressive delivery controller. The same concepts still apply:

- stable and canary run as separate workloads
- traffic percentage is controlled independently from deployment
- feature flags control behavior inside the app
- health gates decide whether to promote or roll back
- kill switches disable risky behavior without rebuilding images

The local Docker Compose version keeps those ideas visible without requiring a cluster.
