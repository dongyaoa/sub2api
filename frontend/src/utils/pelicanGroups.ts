// The same group name can belong to different groups. An older response without
// a group ID keeps its plan separate until the next server upgrade.
export function groupPelicanPlans<T extends { id: number; group_id?: number | null }>(plans: readonly T[]) {
  const groups = new Map<string, { key: string; groupId: number | null; plans: T[] }>()
  for (const plan of plans) {
    const id = plan.group_id
    const groupId = typeof id === 'number' && Number.isSafeInteger(id) && id > 0 ? id : null
    const key = groupId === null ? `plan:${plan.id}` : `group:${groupId}`
    const group = groups.get(key)
    if (group) group.plans.push(plan)
    else groups.set(key, { key, groupId, plans: [plan] })
  }
  return Array.from(groups.values())
}
