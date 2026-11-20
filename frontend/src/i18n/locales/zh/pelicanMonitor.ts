export default {
  pelicanMonitor: {
    artworkModal: {
      group: '{name} · 倍率 {rate}', title: '第 {round} 轮检测 · 组内 {count} 模型同屏画质对比',
      compare: '与组内其余模型同轮对比 ({count} 模型)', single: '切回单张大图',
      effort: '{effort} 思考', elapsed: '耗时 {duration}', model: '模型：', duration: '生成耗时：',
      missing: '本轮暂无作品',
    },
    card: {
      showModel: '点击切换到 {name}', includeModel: '点击勾选/取消参与同时查看',
      single: '单看切换', paired: '并排同看', stacked: '上下同看',
      singleHint: '单排展示当前选中模型，点击模型标签秒切', pairedHint: '不增加卡片高度！将同一轮次的多模型作品左右并排同屏对比', stackedHint: '上下分排对齐展示各模型的 10 次时间轴，支持同步横向滚动',
      left: '向左滚动', right: '向右滚动', latestRound: '★ 最新轮次同屏对比', round: '第 {count} 轮历史对比', enlargeRound: '放大对比 ⤢', open: '点击放大 / 对比', finished: '已完成',
    },
    title: '鹈鹕监测', description: '用同一道绘画题，持续观察各分组内多模型的生成表现，支持一键切换或同屏对比。', notice: '公告',
    aggregated: '多模型同组聚合版', globalLayout: '全局默认排布:', justUpdated: '刚刚更新',
    summary: '{groups} 个分组 · {models} 个模型',
    drawingPrompt: '创建一个 HTML，内容是用 SVG 绘制一个鹈鹕骑自行车的 2D 动画。你不需要任何测试',
    model: '检测模型', reasoning: '思考强度', reasoningShort: '思考',
    rate: '分组倍率', schedule: '检测周期', everyMinutes: '每 {count} 分钟检测一次', everySeconds: '每 {count} 秒检测一次', next: '下次检测', afterCurrent: '本次完成后安排', waitingSchedule: '等待调度', paused: '定时已暂停', lastRun: '最近检测',
    comparison: '对比', primaryModel: '主看模型', compareModel: '选择对比模型', viewAsPrimary: '切到上排',
    recentWorks: '最近作品', newestFirst: '左新右旧', latest: '最新', duration: '生成耗时', search: '搜索分组或模型', groupCount: '{count} 个分组', updated: '{seconds} 秒前更新',
    status: { pending: '等待检测', running: '绘制中', succeeded: '已完成', failed: '生成失败', idle: '尚未检测' },
    refresh: '刷新', preview: '鹈鹕动画预览', open: '查看作品', reloadPreview: '重播', loadingPreview: '载入作品…', hoverPreview: '悬停或聚焦播放，点击放大查看', loadFailed: '暂时无法加载，请稍后刷新重试。', noHTML: '本次没有可展示的作品', waiting: '等待第一幅作品',
    disabled: '鹈鹕监测暂未开放', disabledHint: '开放后可在这里查看各分组的最新作品。', empty: '暂无公开的监测分组', noMatches: '没有匹配的分组', retention: '每个模型保留最近 10 幅作品。上排前 8 幅可见时自动播放；对比小图悬停或聚焦播放，点击可放大。',
  },
}
