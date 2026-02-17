# TODO

## Features



## Performance Optimizations
- [x] Memory optimization with cache.TransformFunc: Use `informers.WithTransform()` to strip unnecessary fields before caching (reduces memory by ~90%)
  - Strip Pod fields: keep only name, namespace, nodeName, phase, container resource requests
  - Strip Node fields: keep only name, labels, and allocatable resources
  - Maintains single watch connection while reducing memory from ~5-10MB to ~500KB for 1000 pods
  - Implementation: `stripUnusedFields()` function that transforms objects before entering informer cache
- [ ] Event-handler based pre-computation: maintain running tallies updated on pod ADDED/MODIFIED/DELETED events instead of iterating all pods on each scrape (O(nodes) scrape instead of O(pods))
- [x] Paginated initial list: use client-go paging support to load pods in batches during initial informer sync
- [ ] Field selector filtering: avoid caching terminated pods that aren't needed for allocation calculation

