export default {
  pelicanMonitor: {
    artworkModal: {
      group: '{name} · Multiplier {rate}', title: 'Round {round} · Compare {count} models in this group',
      compare: 'Compare this round across {count} models', single: 'Back to single artwork',
      effort: '{effort} effort', elapsed: 'Time {duration}', model: 'Model: ', duration: 'Generation time: ',
      missing: 'No artwork for this round',
    },
    card: {
      showModel: 'Switch to {name}', includeModel: 'Include or exclude this model in the comparison',
      single: 'Single model', paired: 'Side by side', stacked: 'Stacked',
      singleHint: 'View one model at a time. Select a model to switch.', pairedHint: 'Compare model artwork side by side for each round.', stackedHint: 'Align the last 10 checks in rows and scroll the model timelines together.',
      left: 'Scroll left', right: 'Scroll right', latestRound: '★ Latest round comparison', round: 'Round {count} comparison', enlargeRound: 'Enlarge comparison ⤢', open: 'Enlarge / compare', finished: 'finished',
    },
    title: 'Pelican monitor', description: 'Follow models within each group using the same drawing prompt. Switch between models or compare them together.', notice: 'Notice',
    aggregated: 'Models grouped together', globalLayout: 'Default layout:', justUpdated: 'Just updated',
    summary: '{groups} groups · {models} models',
    drawingPrompt: 'Create an HTML page with an SVG animation of a pelican riding a bicycle in 2D. No tests are needed.',
    model: 'Model', reasoning: 'Reasoning', reasoningShort: 'effort',
    rate: 'Group multiplier', schedule: 'Schedule', everyMinutes: 'Every {count} minutes', everySeconds: 'Every {count} seconds', next: 'Next check', afterCurrent: 'After this run', waitingSchedule: 'Awaiting schedule', paused: 'Schedule paused', lastRun: 'Last check',
    comparison: 'Compare', primaryModel: 'Primary model', compareModel: 'Choose comparison model', viewAsPrimary: 'Move to top',
    recentWorks: 'Recent artwork', newestFirst: 'Newest first', latest: 'Latest', duration: 'Generation time', search: 'Search groups or models', groupCount: '{count} groups', updated: 'Updated {seconds}s ago',
    status: { pending: 'Waiting', running: 'Drawing', succeeded: 'Completed', failed: 'Generation failed', idle: 'Not checked yet' },
    refresh: 'Refresh', preview: 'Pelican animation preview', open: 'View artwork', reloadPreview: 'Replay', loadingPreview: 'Loading artwork…', hoverPreview: 'Hover or focus to play; tap to enlarge', loadFailed: 'Unable to load. Please refresh and try again.', noHTML: 'No artwork available for this run', waiting: 'Waiting for the first artwork',
    disabled: 'Pelican monitoring is not available', disabledHint: 'Latest group artwork will appear here when monitoring is opened.', empty: 'No monitoring groups are published yet', noMatches: 'No matching groups', retention: 'Each model keeps its 10 latest artworks. The first 8 in the top row play when visible. Hover or focus comparison previews to play, or select an artwork to enlarge it.',
  },
}
