export const PELICAN_ARTWORK_LIMIT = 10

type ArtworkRun = { id: number; status: string; test_kind?: string }

// Keep the server order. During polling the latest run can be newer than, or
// have a fresher status than, the record in the history strip.
export function retainedPelicanArtworks<T extends ArtworkRun>(plan: { latest_run?: T | null; recent_runs?: T[] }): T[] {
  const latest = plan.latest_run
  const recent = plan.recent_runs ?? []
  const completed: T[] = []
  const seen = new Set<number>()
  function retain(run: T) {
    if (run.test_kind === 'candy' || (run.status !== 'succeeded' && run.status !== 'failed') || seen.has(run.id)) return
    seen.add(run.id)
    completed.push(run)
  }
  if (latest && !recent.some(run => run.id === latest.id)) retain(latest)
  for (const run of recent) {
    retain(latest?.id === run.id ? latest : run)
    if (completed.length === PELICAN_ARTWORK_LIMIT) break
  }
  return completed
}
