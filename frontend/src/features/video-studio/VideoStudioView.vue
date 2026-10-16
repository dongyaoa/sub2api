<template>
  <AppLayout>
    <div class="mx-auto w-full max-w-[1920px]">
      <MediaStudioHeader active="video" />

      <div class="video-workspace grid grid-cols-1 items-stretch gap-4 xl:grid-cols-[380px_minmax(0,1fr)] 2xl:grid-cols-[380px_minmax(0,1fr)_260px]">
        <aside class="flex min-w-0 flex-col">
          <div class="mb-2 flex min-h-8 items-center justify-between gap-3">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('videoStudio.settings') }}</h2>
            <span
              v-if="selectedGroup"
              class="rounded-md bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300"
            >
              {{ t('videoStudio.estimate', { amount: formatUSD(estimatedCost) }) }}
            </span>
          </div>

          <section class="video-settings-panel flex-1 rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900">
            <form class="space-y-3" @submit.prevent="generateVideo">
              <div class="grid grid-cols-2 gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-800" role="tablist" :aria-label="t('videoStudio.operationMode')">
                <button
                  type="button"
                  class="flex h-9 items-center justify-center gap-1.5 rounded-md px-2 text-xs font-medium transition-colors"
                  :class="operation === 'text' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
                  :disabled="isWorking"
                  :aria-selected="operation === 'text'"
                  role="tab"
                  @click="switchOperation('text')"
                >
                  <Icon name="sparkles" size="xs" />
                  {{ t('videoStudio.textMode') }}
                </button>
                <button
                  type="button"
                  class="flex h-9 items-center justify-center gap-1.5 rounded-md px-2 text-xs font-medium transition-colors"
                  :class="operation === 'image' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
                  :disabled="isWorking"
                  :aria-selected="operation === 'image'"
                  role="tab"
                  @click="switchOperation('image')"
                >
                  <Icon name="play" size="xs" />
                  {{ t('videoStudio.imageMode') }}
                </button>
              </div>

              <div v-if="loadingKeys" class="flex h-10 items-center justify-center rounded-lg border border-gray-200 text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <Icon name="refresh" size="sm" class="mr-2 animate-spin" />
                {{ t('videoStudio.loadingKeys') }}
              </div>

              <div v-else-if="apiKeys.length === 0" class="flex h-10 items-center justify-between gap-3 rounded-lg border border-amber-200 bg-amber-50 px-3 dark:border-amber-900/60 dark:bg-amber-950/20">
                <span class="truncate text-xs text-amber-800 dark:text-amber-300">{{ t('videoStudio.noKeys') }}</span>
                <RouterLink to="/keys" class="inline-flex shrink-0 items-center gap-1 text-xs font-semibold text-primary-600 hover:text-primary-700 dark:text-primary-400">
                  <Icon name="plus" size="xs" />
                  {{ t('videoStudio.createKey') }}
                </RouterLink>
              </div>

              <div v-else>
                <label class="input-label" for="video-studio-key">{{ t('videoStudio.apiKey') }}</label>
                <Select
                  id="video-studio-key"
                  v-model="selectedKeyId"
                  :options="keyOptions"
                  :disabled="isWorking"
                  :placeholder="t('videoStudio.selectKey')"
                  @change="handleKeySelection"
                />
              </div>

              <div>
                <div class="mb-1 flex items-center justify-between gap-2">
                  <label class="text-sm font-medium text-gray-700 dark:text-gray-300" for="video-studio-model">{{ t('videoStudio.model') }}</label>
                  <span v-if="loadingModels" class="flex items-center gap-1 text-xs text-gray-400">
                    <Icon name="refresh" size="xs" class="animate-spin" />
                    {{ t('videoStudio.loading') }}
                  </span>
                </div>
                <Select
                  id="video-studio-model"
                  v-model="model"
                  :options="modelOptions"
                  :disabled="isWorking || loadingModels || !selectedKey"
                  :placeholder="t('videoStudio.selectModel')"
                  :empty-text="t('videoStudio.noModels')"
                  @change="handleModelSelection"
                />
              </div>

              <div v-if="operation === 'image'">
                <div class="mb-1 flex items-center justify-between gap-2">
                  <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('videoStudio.sourceImage') }}</span>
                  <span class="text-[11px] text-gray-400">{{ t('videoStudio.sourceImageLimit') }}</span>
                </div>
                <input
                  ref="sourceInput"
                  type="file"
                  class="sr-only"
                  accept=".png,.jpg,.jpeg,.webp,image/png,image/jpeg,image/webp"
                  :disabled="isWorking"
                  @change="handleSourceInput"
                />
                <div
                  v-if="sourcePreviewUrl"
                  class="flex h-24 items-center gap-3 overflow-hidden rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-700 dark:bg-dark-800"
                >
                  <img :src="sourcePreviewUrl" :alt="t('videoStudio.sourcePreview')" class="h-20 w-20 shrink-0 rounded-md object-cover" />
                  <div class="min-w-0 flex-1">
                    <p class="truncate text-xs font-medium text-gray-800 dark:text-gray-200" :title="sourceImageFile?.name || sourceImageUrl">
                      {{ sourceImageFile?.name || t('videoStudio.generatedSource') }}
                    </p>
                    <p class="mt-1 text-[11px] text-gray-400">
                      {{ sourceImageFile ? formatFileSize(sourceImageFile.size) : t('videoStudio.ready') }}
                    </p>
                  </div>
                  <div class="flex shrink-0 gap-1">
                    <button type="button" class="flex h-8 w-8 items-center justify-center rounded-md text-gray-500 hover:bg-white hover:text-primary-600 disabled:opacity-40 dark:hover:bg-dark-700 dark:hover:text-primary-400" :disabled="isWorking" :title="t('videoStudio.replaceImage')" @click="openSourcePicker">
                      <Icon name="upload" size="sm" />
                    </button>
                    <button type="button" class="flex h-8 w-8 items-center justify-center rounded-md text-gray-500 hover:bg-white hover:text-red-600 disabled:opacity-40 dark:hover:bg-dark-700 dark:hover:text-red-400" :disabled="isWorking" :title="t('videoStudio.removeImage')" @click="clearSourceImage">
                      <Icon name="trash" size="sm" />
                    </button>
                  </div>
                </div>
                <button
                  v-else
                  type="button"
                  class="flex h-24 w-full items-center justify-center gap-3 rounded-lg border border-dashed px-3 text-left transition-colors"
                  :class="sourceDragActive ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-950/20 dark:text-primary-300' : 'border-gray-300 bg-gray-50 text-gray-500 hover:border-primary-400 hover:text-primary-600 dark:border-dark-600 dark:bg-dark-800 dark:text-gray-400'"
                  :disabled="isWorking"
                  @click="openSourcePicker"
                  @dragenter.prevent="sourceDragActive = true"
                  @dragover.prevent="sourceDragActive = true"
                  @dragleave.prevent="sourceDragActive = false"
                  @drop.prevent="handleSourceDrop"
                >
                  <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-current/20 bg-white dark:bg-dark-900">
                    <Icon name="upload" size="md" />
                  </span>
                  <span class="min-w-0">
                    <span class="block text-xs font-semibold">{{ t('videoStudio.uploadSource') }}</span>
                    <span class="mt-0.5 block text-[11px] opacity-75">{{ t('videoStudio.uploadSourceHint') }}</span>
                  </span>
                </button>
              </div>

              <div>
                <div class="mb-1 flex items-center justify-between gap-2">
                  <label class="text-sm font-medium text-gray-700 dark:text-gray-300" for="video-studio-prompt">{{ t('videoStudio.prompt') }}</label>
                  <span class="text-xs tabular-nums text-gray-400">{{ prompt.length }}</span>
                </div>
                <textarea
                  id="video-studio-prompt"
                  v-model="prompt"
                  :disabled="isWorking"
                  :placeholder="operation === 'image' ? t('videoStudio.imagePromptPlaceholder') : t('videoStudio.promptPlaceholder')"
                  rows="4"
                  class="input min-h-[92px] resize-none"
                />
              </div>

              <div>
                <div class="mb-1.5 flex items-center justify-between gap-3">
                  <label class="text-sm font-medium text-gray-700 dark:text-gray-300" for="video-studio-duration">{{ t('videoStudio.duration') }}</label>
                  <span class="text-sm font-semibold tabular-nums text-gray-900 dark:text-white">{{ t('videoStudio.seconds', { count: duration }) }}</span>
                </div>
                <input
                  id="video-studio-duration"
                  v-model.number="duration"
                  type="range"
                  min="1"
                  max="15"
                  step="1"
                  class="h-2 w-full cursor-pointer accent-primary-600 disabled:cursor-not-allowed disabled:opacity-50"
                  :disabled="isWorking"
                />
                <div class="mt-1 flex justify-between text-[11px] tabular-nums text-gray-400">
                  <span>1s</span>
                  <span>8s</span>
                  <span>15s</span>
                </div>
              </div>

              <div>
                <span class="input-label">{{ t('videoStudio.resolution') }}</span>
                <div class="grid grid-cols-3 gap-2">
                  <button
                    v-for="tier in allResolutions"
                    :key="tier"
                    type="button"
                    class="h-9 rounded-md border text-xs font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-40"
                    :class="resolution === tier ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-950/30 dark:text-primary-300' : 'border-gray-200 text-gray-600 hover:border-gray-300 dark:border-dark-600 dark:text-gray-300 dark:hover:border-dark-500'"
                    :disabled="isWorking || !resolutionOptions.includes(tier)"
                    @click="resolution = tier"
                  >
                    {{ tier }}
                  </button>
                </div>
              </div>

              <div>
                <span class="input-label">{{ t('videoStudio.aspectRatio') }}</span>
                <div class="grid grid-cols-4 gap-1.5">
                  <button
                    v-for="ratio in aspectRatioOptions"
                    :key="ratio"
                    type="button"
                    class="h-8 rounded-md border text-xs font-semibold tabular-nums transition-colors"
                    :class="aspectRatio === ratio ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-950/30 dark:text-primary-300' : 'border-gray-200 text-gray-600 hover:border-gray-300 dark:border-dark-600 dark:text-gray-300 dark:hover:border-dark-500'"
                    :disabled="isWorking"
                    @click="aspectRatio = ratio"
                  >
                    {{ ratio }}
                  </button>
                </div>
              </div>

              <section v-if="selectedGroup" class="border-y border-gray-200 py-2.5 dark:border-dark-700" :aria-label="t('videoStudio.pricing')">
                <div class="mb-2 flex items-center justify-between gap-2">
                  <span class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('videoStudio.pricing') }}</span>
                  <span class="max-w-[150px] truncate text-[11px] text-gray-400">{{ selectedGroup.name }}</span>
                </div>
                <div class="grid gap-1.5" :class="visiblePriceTiers.length === 3 ? 'grid-cols-3' : 'grid-cols-2'">
                  <div
                    v-for="price in visiblePriceTiers"
                    :key="price.resolution"
                    class="min-w-0 rounded-md border px-2 py-1.5"
                    :class="resolution === price.resolution ? 'border-primary-400 bg-primary-50/60 dark:bg-primary-950/20' : 'border-gray-200 dark:border-dark-700'"
                  >
                    <div class="flex items-center justify-between gap-1">
                      <span class="text-[11px] font-semibold text-gray-700 dark:text-gray-300">{{ price.resolution }}</span>
                      <span class="h-1.5 w-1.5 rounded-full" :class="price.configured ? 'bg-emerald-500' : 'bg-gray-300 dark:bg-gray-600'" />
                    </div>
                    <div class="truncate text-sm font-semibold tabular-nums text-gray-950 dark:text-white">{{ formatUSD(price.unitPrice) }}/s</div>
                  </div>
                </div>
              </section>

              <button
                type="submit"
                class="flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-primary-600 px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500/40 focus:ring-offset-2 disabled:cursor-not-allowed disabled:bg-gray-300 dark:disabled:bg-dark-700"
                :disabled="!canGenerate"
              >
                <Icon :name="isWorking ? 'refresh' : 'play'" size="md" :class="isWorking ? 'animate-spin' : ''" />
                <span>{{ submitting ? t('videoStudio.submitting') : t('videoStudio.generate') }}</span>
                <span v-if="!isWorking && selectedGroup" class="text-white/75">{{ formatUSD(estimatedCost) }}</span>
              </button>
              <p class="text-center text-[11px] leading-5 text-gray-400">{{ t('videoStudio.multitaskHint') }}</p>
            </form>
          </section>
        </aside>

        <main class="flex min-w-0 flex-col">
          <div class="mb-2 flex min-h-8 flex-wrap items-center justify-between gap-3">
            <div class="flex min-w-0 flex-wrap items-center gap-2">
              <h2 class="shrink-0 text-sm font-semibold text-gray-900 dark:text-white">{{ t('videoStudio.canvas') }}</h2>
              <span v-if="runningCount" class="rounded-md bg-amber-50 px-2 py-1 text-[11px] font-medium text-amber-700 dark:bg-amber-950/30 dark:text-amber-300">{{ t('videoStudio.runningTasks', { count: runningCount }) }}</span>
            </div>
            <button type="button" data-testid="new-task" class="ml-auto inline-flex h-7 shrink-0 items-center gap-1 rounded-md border border-emerald-200 bg-emerald-50 px-2.5 text-xs font-semibold text-emerald-700 transition-colors hover:border-emerald-300 hover:bg-emerald-100 focus:outline-none focus:ring-2 focus:ring-emerald-500/25 disabled:cursor-not-allowed disabled:opacity-40 dark:border-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-300 dark:hover:bg-emerald-900/50" :disabled="isWorking" @click="newTask">
              <Icon name="plus" size="xs" />{{ t('videoStudio.newTask') }}
            </button>
          </div>

          <section class="video-canvas relative flex min-h-0 flex-col overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
            <div v-if="viewState !== 'idle'" data-testid="task-status" class="flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 border-b border-gray-100 px-4 py-2.5 text-xs dark:border-dark-700">
              <span class="flex min-w-0 items-center gap-1.5 font-medium text-gray-700 dark:text-gray-300"><span class="h-1.5 w-1.5 shrink-0 rounded-full" :class="statusDotClass" />{{ statusLabel }}</span>
              <span class="whitespace-nowrap tabular-nums text-gray-400">{{ t('videoStudio.elapsedTime') }} {{ formatElapsed(elapsedSeconds) }}</span>
              <span v-if="viewState === 'processing' && taskProgress != null" class="tabular-nums text-gray-400">{{ taskProgress }}%</span>
              <span v-if="requestId" class="ml-auto min-w-0 max-w-[180px] truncate text-[11px] text-gray-400" :title="requestId">{{ requestId }}</span>
            </div>
            <div v-if="viewState === 'idle'" class="flex min-h-0 flex-1 flex-col items-center justify-center overflow-y-auto px-6 py-4 text-center">
              <div class="mb-4 flex h-14 w-14 items-center justify-center rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800">
                <Icon name="play" size="xl" class="text-gray-400 dark:text-gray-500" />
              </div>
              <p class="text-base font-medium text-gray-800 dark:text-gray-200">{{ t('videoStudio.empty') }}</p>
              <p class="mt-1 text-sm text-gray-400">{{ t('videoStudio.emptyHint') }}</p>
            </div>

            <div v-else-if="viewState === 'processing'" class="flex min-h-0 flex-1 flex-col items-center justify-center overflow-y-auto px-6 py-4 text-center">
              <span class="mb-5 flex h-14 w-14 items-center justify-center rounded-lg border border-primary-200 bg-primary-50 text-primary-600 dark:border-primary-900 dark:bg-primary-950/30 dark:text-primary-400">
                <Icon name="play" size="xl" class="animate-pulse" />
              </span>
              <p class="text-base font-semibold text-gray-900 dark:text-white">{{ statusLabel }}</p>
              <p class="mt-2 text-sm tabular-nums text-gray-400">{{ formatElapsed(elapsedSeconds) }}</p>
              <div class="mt-5 h-2 w-full max-w-md overflow-hidden rounded-full bg-gray-100 dark:bg-dark-800">
                <div
                  class="h-full rounded-full bg-primary-500 transition-[width] duration-500"
                  :class="taskProgress == null ? 'video-indeterminate-progress' : ''"
                  :style="taskProgress == null ? undefined : { width: taskProgress + '%' }"
                />
              </div>
              <p v-if="taskProgress != null" class="mt-2 text-xs tabular-nums text-gray-400">{{ taskProgress }}%</p>
              <p v-if="isLongRunning" class="mt-4 max-w-md text-sm leading-6 text-gray-500 dark:text-gray-400">
                {{ t('videoStudio.backgroundProcessingHint') }}
              </p>
              <p v-if="pollWarning" class="mt-2 max-w-md text-xs leading-5 text-amber-600 dark:text-amber-400" :title="pollWarning">
                {{ t('videoStudio.pollRetrying') }}
              </p>
            </div>

            <div v-else-if="viewState === 'failed'" class="flex min-h-0 flex-1 flex-col items-center justify-center overflow-y-auto px-6 py-4 text-center">
              <div class="mb-4 flex h-14 w-14 items-center justify-center rounded-lg bg-red-50 text-red-600 dark:bg-red-950/30 dark:text-red-400">
                <Icon name="exclamationCircle" size="xl" />
              </div>
              <p class="text-base font-semibold text-gray-900 dark:text-white">{{ t('videoStudio.failed') }}</p>
              <p class="mt-2 max-w-xl break-words text-sm leading-6 text-gray-500 dark:text-gray-400">{{ errorMessage }}</p>
              <button type="button" class="mt-5 inline-flex items-center gap-2 rounded-lg border border-gray-200 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-800" :disabled="isWorking || loadingModels" @click="reuseParameters">
                <Icon name="refresh" size="sm" />
                {{ t('videoStudio.reuseParameters') }}
              </button>
            </div>

            <div v-else class="flex min-h-0 flex-1 flex-col p-3">
              <div class="video-player-frame relative flex min-h-0 min-w-0 flex-1 items-center justify-center overflow-hidden rounded-lg bg-black">
                <div v-if="loadingContent" class="flex flex-col items-center text-white/80">
                  <Icon name="refresh" size="xl" class="mb-3 animate-spin" />
                  <span class="text-sm">{{ t('videoStudio.loadingVideo') }}</span>
                </div>
                <template v-else-if="videoObjectUrl">
                  <video
                    ref="videoPlayer"
                    :src="videoObjectUrl"
                    class="absolute inset-0 h-full w-full object-contain"
                    controls
                    playsinline
                    preload="auto"
                    @canplay="handleVideoCanPlay"
                    @canplaythrough="handleVideoCanPlay"
                    @playing="handleVideoPlaying"
                    @pause="videoBuffering = false"
                    @waiting="handleVideoWaiting"
                    @stalled="handleVideoWaiting"
                    @error="handleVideoPlaybackError"
                  />
                  <div v-if="!videoReady" class="absolute inset-0 flex flex-col items-center justify-center bg-black/35 text-white/90">
                    <Icon name="refresh" size="xl" class="mb-3 animate-spin" />
                    <span class="text-sm">{{ t('videoStudio.preparingVideo') }}</span>
                  </div>
                  <button
                    v-else-if="!videoStarted"
                    type="button"
                    class="absolute flex h-16 w-16 items-center justify-center rounded-full border border-white/40 bg-black/55 text-white shadow-lg transition-colors hover:bg-black/70 disabled:cursor-wait"
                    :disabled="videoPlayPending"
                    :title="t('videoStudio.playVideo')"
                    :aria-label="t('videoStudio.playVideo')"
                    @click="playVideo"
                  >
                    <Icon :name="videoPlayPending ? 'refresh' : 'play'" size="xl" :class="videoPlayPending ? 'animate-spin' : 'ml-0.5'" />
                  </button>
                  <div v-if="videoStarted && videoBuffering" class="pointer-events-none absolute inset-0 flex items-center justify-center bg-black/20 text-white">
                    <span class="flex h-12 w-12 items-center justify-center rounded-full bg-black/55">
                      <Icon name="refresh" size="lg" class="animate-spin" />
                    </span>
                  </div>
                </template>
                <div v-else class="flex max-w-lg flex-col items-center px-6 text-center text-white/80">
                  <Icon name="exclamationCircle" size="xl" class="mb-3" />
                  <p class="text-sm leading-6">{{ contentError || t('videoStudio.contentUnavailable') }}</p>
                  <button type="button" class="mt-4 inline-flex items-center gap-2 rounded-md border border-white/25 px-3 py-2 text-sm font-medium text-white hover:bg-white/10" :disabled="clearingHistory" @click="reloadVideoContent">
                    <Icon name="refresh" size="sm" />
                    {{ t('videoStudio.reloadContent') }}
                  </button>
                </div>
              </div>
              <div class="flex min-h-14 shrink-0 flex-wrap items-center justify-between gap-3 px-2 pt-3">
                <div class="text-xs text-gray-400">
                  {{ displayModel }} · {{ displayResolution }}<template v-if="displayAspectRatio"> · {{ displayAspectRatio }}</template> · {{ t('videoStudio.seconds', { count: displayDuration }) }}
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <button type="button" class="inline-flex h-8 items-center gap-1.5 rounded-md border border-gray-200 px-2.5 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-40 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-800" :disabled="isWorking || loadingModels" @click="reuseParameters">
                    <Icon name="refresh" size="xs" />{{ t('videoStudio.reuseParameters') }}
                  </button>
                  <button type="button" class="inline-flex h-8 items-center gap-1.5 rounded-md border border-gray-200 px-2.5 text-xs font-medium text-gray-700 hover:bg-gray-50 disabled:opacity-40 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-800" :disabled="loadingContent || clearingHistory" @click="downloadVideo">
                    <Icon name="download" size="xs" />{{ t('videoStudio.download') }}
                  </button>
                </div>
              </div>
            </div>
          </section>
        </main>

        <aside class="video-history flex min-w-0 flex-col xl:col-span-2 2xl:col-span-1">
          <div class="mb-2 flex min-h-8 items-center justify-between gap-2">
            <div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('videoStudio.history') }}</h2>
              <span class="text-xs tabular-nums text-gray-400">{{ history.length }}</span>
              <span class="text-[11px] text-gray-400">{{ persistentHistory ? t('videoStudio.historyPermanent') : t('videoStudio.historyRetention', { count: retentionDays }) }}</span>
            </div>
            <button
              type="button"
              class="flex h-8 w-8 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-30 dark:hover:bg-dark-800 dark:hover:text-gray-200"
              :disabled="history.length === 0 || isWorking || loadingHistory || runningCount > 0 || deliveringCount > 0"
              :title="runningCount || deliveringCount ? t('videoStudio.clearHistoryBusy') : t('videoStudio.clearHistory')"
              :aria-label="t('videoStudio.clearHistory')"
              @click="clearHistory"
            >
              <Icon name="trash" size="sm" />
            </button>
          </div>

          <section class="video-history-panel flex-1 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
            <div v-if="history.length === 0" class="flex min-h-40 flex-col items-center justify-center px-4 text-center text-gray-400">
              <Icon name="clock" size="lg" class="mb-2" />
              <p class="text-xs font-medium">{{ loadingHistory ? t('videoStudio.loading') : t('videoStudio.historyEmpty') }}</p>
              <p class="mt-1 text-[11px]">{{ t('videoStudio.historyHint') }}</p>
            </div>

            <div v-else class="video-history-list divide-y divide-gray-100 overflow-y-auto dark:divide-dark-700">
              <button
                v-for="item in history"
                :key="item.requestId"
                type="button"
                class="grid w-full min-w-0 grid-cols-[auto_minmax(0,1fr)] gap-x-2.5 gap-y-2 overflow-hidden border-l-2 px-3 py-3 text-left transition-colors"
                :class="activeHistoryId === item.requestId ? 'border-l-primary-500 bg-primary-50/60 dark:bg-primary-950/20' : 'border-l-transparent hover:bg-gray-50 dark:hover:bg-dark-800'"
                :title="item.request.prompt"
                :disabled="clearingHistory"
                @click="selectHistoryItem(item)"
              >
                <div data-testid="history-meta" class="col-span-2 grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-1 text-[10px] leading-4">
                  <div data-testid="history-status" class="flex min-w-0 items-center gap-1" :class="historyStatusClass(item.status)">
                    <span class="h-1.5 w-1.5 shrink-0 rounded-full" :class="historyDotClass(item.status)" />
                    <span class="truncate">{{ historyStatusLabel(item.status) }}</span>
                    <span class="shrink-0 whitespace-nowrap tabular-nums text-gray-400" :title="t('videoStudio.elapsedTime')">· {{ formatElapsed(videoElapsedSeconds(item, now)) }}</span>
                  </div>
                  <time class="justify-self-end whitespace-nowrap tabular-nums text-gray-400" :datetime="new Date(item.startedAt).toISOString()">{{ formatVideoHistoryTime(item.startedAt) }}</time>
                </div>
                <div class="flex h-12 w-12 shrink-0 items-center justify-center rounded-md bg-gray-100 dark:bg-dark-800">
                  <Icon v-if="item.status === 'failed'" name="exclamationCircle" size="md" class="text-red-500" />
                  <Icon v-else name="play" size="md" :class="item.status === 'processing' ? 'animate-pulse text-primary-500' : 'text-emerald-500'" />
                </div>
                <div class="min-w-0 flex-1">
                  <p class="truncate text-xs font-medium text-gray-800 dark:text-gray-200">{{ item.request.prompt }}</p>
                  <p class="mt-1 truncate text-[11px] text-gray-400">{{ operationLabel(item.operation) }} · {{ item.request.resolution }}<template v-if="item.request.aspect_ratio"> · {{ item.request.aspect_ratio }}</template> · {{ item.request.duration }}s</p>
                  <p class="mt-1 truncate text-[10px] text-gray-400" :title="item.request.model + ' · ' + item.groupName">{{ item.request.model }} · {{ item.groupName }}</p>
                  <p v-if="item.delivering" class="mt-1 text-[10px] text-gray-400">{{ t('videoStudio.savingVideo') }}</p>
                  <p v-else-if="item.deliveryError" class="mt-1 truncate text-[10px] text-amber-600" :title="item.deliveryError">{{ t('videoStudio.saveVideoFailed') }}</p>
                </div>
              </button>
            </div>
          </section>
        </aside>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { userGroupsAPI } from '@/api/groups'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import MediaStudioHeader from '@/features/media-studio/MediaStudioHeader.vue'
import { useAppStore } from '@/stores/app'
import type { ApiKey } from '@/types'
import { listEligibleVideoKeys } from './access'
import { listVideoModels } from './api'
import {
  buildGenerateVideoRequest,
  DEFAULT_VIDEO_ASPECT_RATIO,
  filterVideoModels,
  getVideoResolutionOptions,
  normalizeVideoAspectRatio,
  normalizeVideoResolution,
  selectPreferredVideoModel,
  VIDEO_ASPECT_RATIOS,
} from './capabilities'
import { consumeVideoStudioImageDraft } from './draft'
import { estimateVideoCost, formatUSD, getVideoPriceTiers } from './pricing'
import { LONG_VIDEO_TASK_THRESHOLD_MS } from './polling'
import { formatVideoHistoryTime, useVideoTasks, videoElapsedSeconds, type VideoHistoryItem } from './useVideoTasks'
import type {
  StoredVideoHistoryItem, VideoAspectRatio, VideoModel, VideoResolution,
  VideoStudioError, VideoStudioOperation,
} from './types'

const MAX_SOURCE_IMAGE_BYTES = 20 * 1024 * 1024
const SOURCE_IMAGE_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp'])

const { t } = useI18n()
const appStore = useAppStore()

const operation = ref<VideoStudioOperation>('text')
const sourceImageFile = ref<File | null>(null)
const sourceImageUrl = ref('')
const sourcePreviewUrl = ref('')
const sourceInput = ref<HTMLInputElement | null>(null)
const sourceDragActive = ref(false)
const apiKeys = ref<ApiKey[]>([])
const userRates = ref<Record<number, number>>({})
const selectedKeyId = ref<number | null>(null)
const models = ref<VideoModel[]>([])
const model = ref('')
const prompt = ref('')
const resolution = ref<VideoResolution>('480p')
const aspectRatio = ref<VideoAspectRatio>(DEFAULT_VIDEO_ASPECT_RATIO)
const duration = ref(4)
const loadingKeys = ref(true)
const loadingModels = ref(false)
const tasks = useVideoTasks(apiKeys, {
  error: message => appStore.showError(message || t('videoStudio.historyLoadFailed')),
  missingId: () => t('videoStudio.missingTaskId'),
  failed: () => t('videoStudio.generateFailed'),
})
const { history, activeHistoryId, activeHistoryItem, runningCount, submitting,
  loadingHistory, clearingHistory, deliveringCount, retentionDays, persistentHistory } = tasks
const task = computed(() => activeHistoryItem.value?.task || null)
const requestId = computed(() => activeHistoryItem.value?.requestId || '')
const viewState = computed(() => activeHistoryItem.value?.status || 'idle')
const errorMessage = computed(() => activeHistoryItem.value?.errorMessage || '')
const pollWarning = computed(() => activeHistoryItem.value?.pollWarning || '')
const contentError = ref('')
const loadingContent = ref(false)
const videoObjectUrl = ref('')
const videoPlayer = ref<HTMLVideoElement | null>(null)
const videoReady = ref(false)
const videoStarted = ref(false)
const videoBuffering = ref(false)
const videoPlayPending = ref(false)
const now = ref(Date.now())
let disposed = false
let modelController: AbortController | null = null
let contentController: AbortController | null = null
let elapsedTimer: ReturnType<typeof setInterval> | null = null

const selectedKey = computed(() => apiKeys.value.find((key) => key.id === selectedKeyId.value) || null)
const selectedGroup = computed(() => selectedKey.value?.group || null)
const keyOptions = computed(() => apiKeys.value.map((key) => ({
  value: key.id,
  label: key.name + ' · ' + (key.group?.name || '-'),
})))
const modelOptions = computed(() => models.value.map((item) => ({
  value: item.id,
  label: selectedGroup.value?.platform === 'grok'
    ? item.id
    : item.display_name && item.display_name !== item.id
    ? item.display_name + ' (' + item.id + ')'
    : item.id,
})))
const allResolutions: VideoResolution[] = ['480p', '720p', '1080p']
const aspectRatioOptions = VIDEO_ASPECT_RATIOS
const resolutionOptions = computed(() => getVideoResolutionOptions(model.value))
const priceTiers = computed(() => {
  if (!selectedGroup.value) return []
  return getVideoPriceTiers(selectedGroup.value, model.value, userRates.value[selectedGroup.value.id])
})
const visiblePriceTiers = computed(() => priceTiers.value.filter((item) => resolutionOptions.value.includes(item.resolution)))
const estimatedCost = computed(() => {
  if (!selectedGroup.value) return 0
  return estimateVideoCost(
    selectedGroup.value,
    model.value,
    resolution.value,
    duration.value,
    userRates.value[selectedGroup.value.id],
  )
})
// Only lock the draft during submission; accepted jobs run independently.
const isWorking = computed(() => submitting.value || clearingHistory.value)
const hasSourceImage = computed(() => Boolean(sourceImageFile.value || sourceImageUrl.value.trim()))
const canGenerate = computed(() => (
  !isWorking.value
  && !loadingModels.value
  && Boolean(selectedKey.value)
  && Boolean(model.value.trim())
  && Boolean(prompt.value.trim())
  && (operation.value === 'text' || hasSourceImage.value)
))
const elapsedSeconds = computed(() => activeHistoryItem.value ? videoElapsedSeconds(activeHistoryItem.value, now.value) : null)
const isLongRunning = computed(() => viewState.value === 'processing' && (elapsedSeconds.value || 0) * 1000 >= LONG_VIDEO_TASK_THRESHOLD_MS)
const taskProgress = computed(() => {
  const progress = task.value?.progress
  if (progress == null) return null
  const value = Number(progress)
  return Number.isFinite(value) ? Math.min(100, Math.max(0, Math.round(value))) : null
})
const statusLabel = computed(() => {
  if (viewState.value === 'processing' && !task.value) return t('videoStudio.submitting')
  if (viewState.value === 'processing') return t(isLongRunning.value ? 'videoStudio.backgroundProcessing' : 'videoStudio.processing')
  if (viewState.value === 'completed') return t('videoStudio.completed')
  return t('videoStudio.failed')
})
const statusDotClass = computed(() => {
  if (viewState.value === 'completed') return 'bg-emerald-500'
  if (viewState.value === 'failed') return 'bg-red-500'
  return 'animate-pulse bg-amber-500'
})
const displayModel = computed(() => activeHistoryItem.value?.request.model || model.value)
const displayResolution = computed(() => activeHistoryItem.value?.request.resolution || resolution.value)
const displayAspectRatio = computed(() => activeHistoryItem.value
  ? activeHistoryItem.value.request.aspect_ratio || ''
  : aspectRatio.value)
const displayDuration = computed(() => activeHistoryItem.value?.request.duration || duration.value)

function switchOperation(nextOperation: VideoStudioOperation) {
  if (isWorking.value || operation.value === nextOperation) return
  operation.value = nextOperation
  model.value = selectPreferredVideoModel(models.value, nextOperation, model.value)
  resolution.value = normalizeVideoResolution(model.value, resolution.value)
}

function handleModelSelection() {
  resolution.value = normalizeVideoResolution(model.value, resolution.value)
}

function lastKeyStorageKey() {
  return 'video_studio_last_key_v1:' + (apiKeys.value[0]?.user_id || 'local')
}

async function handleKeySelection() {
  try { localStorage.setItem(lastKeyStorageKey(), String(selectedKeyId.value || '')) } catch { /* Optional preference. */ }
  await loadModels()
}

function openSourcePicker() {
  sourceInput.value?.click()
}

function isSupportedSourceImage(file: File): boolean {
  if (!SOURCE_IMAGE_TYPES.has(file.type)) {
    appStore.showError(t('videoStudio.sourceImageTypeError'))
    return false
  }
  if (file.size > MAX_SOURCE_IMAGE_BYTES) {
    appStore.showError(t('videoStudio.sourceImageSizeError'))
    return false
  }
  return true
}

function releaseSourcePreview() {
  if (sourcePreviewUrl.value.startsWith('blob:')) URL.revokeObjectURL(sourcePreviewUrl.value)
}

function setSourceImage(file: File) {
  if (isWorking.value || !isSupportedSourceImage(file)) return
  releaseSourcePreview()
  sourceImageFile.value = file
  sourceImageUrl.value = ''
  sourcePreviewUrl.value = URL.createObjectURL(file)
  sourceDragActive.value = false
}

function setSourceImageURL(url: string) {
  const normalized = url.trim()
  if (!normalized) return
  releaseSourcePreview()
  sourceImageFile.value = null
  sourceImageUrl.value = normalized
  sourcePreviewUrl.value = normalized
  operation.value = 'image'
}

function handleSourceInput(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (file) setSourceImage(file)
}

function handleSourceDrop(event: DragEvent) {
  sourceDragActive.value = false
  const file = event.dataTransfer?.files?.[0]
  if (file) setSourceImage(file)
}

function clearSourceImage() {
  if (isWorking.value) return
  releaseSourcePreview()
  sourceImageFile.value = null
  sourceImageUrl.value = ''
  sourcePreviewUrl.value = ''
}

function formatFileSize(bytes: number): string {
  if (bytes < 1024 * 1024) return Math.max(1, Math.round(bytes / 1024)) + ' KB'
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
}

function fileToDataURL(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result || ''))
    reader.onerror = () => reject(reader.error || new Error('Failed to read image'))
    reader.readAsDataURL(file)
  })
}

async function loadModels(preferredModel = '') {
  modelController?.abort()
  models.value = []
  const key = selectedKey.value
  if (!key) {
    model.value = ''
    loadingModels.value = false
    return
  }
  const controller = new AbortController()
  modelController = controller
  loadingModels.value = true
  try {
    const response = await listVideoModels(key.key, controller.signal)
    if (disposed || modelController !== controller || selectedKeyId.value !== key.id) return
    const filtered = filterVideoModels(response.data || [])
    models.value = filtered
    model.value = selectPreferredVideoModel(filtered, operation.value, preferredModel || model.value)
    resolution.value = normalizeVideoResolution(model.value, resolution.value)
  } catch (error) {
    if (!disposed && modelController === controller && (error as Error).name !== 'AbortError') {
      model.value = ''
      appStore.showError(errorText(error, t('videoStudio.modelsFailed')))
    }
  } finally {
    if (modelController === controller) {
      loadingModels.value = false
      modelController = null
    }
  }
}

function resetVideoPlaybackState() {
  videoPlayer.value = null
  videoReady.value = false
  videoStarted.value = false
  videoBuffering.value = false
  videoPlayPending.value = false
}

function releaseVideoObjectURL() {
  if (videoObjectUrl.value) URL.revokeObjectURL(videoObjectUrl.value)
  videoObjectUrl.value = ''
  resetVideoPlaybackState()
}

function clearVideoContent() {
  contentController?.abort()
  contentController = null
  releaseVideoObjectURL()
  contentError.value = ''
  loadingContent.value = false
}

function newTask() {
  if (isWorking.value) return
  activeHistoryId.value = ''
  prompt.value = ''
  clearSourceImage()
  clearVideoContent()
}

async function generateVideo() {
  if (!canGenerate.value || !selectedKey.value) return
  const key = selectedKey.value
  const mode = operation.value
  const sourceFile = sourceImageFile.value
  const sourceURL = sourceImageUrl.value
  const request = buildGenerateVideoRequest({
    operation: mode, model: model.value, prompt: prompt.value,
    resolution: resolution.value, aspectRatio: aspectRatio.value, duration: duration.value,
  })
  now.value = Date.now()
  await tasks.submit(key, mode, request, mode === 'image'
    ? () => sourceFile ? fileToDataURL(sourceFile) : Promise.resolve(sourceURL)
    : undefined)
}

async function loadVideoContent(key: ApiKey, id: string, refresh = false) {
  clearVideoContent()
  loadingContent.value = true
  const controller = new AbortController()
  contentController = controller
  try {
    const item = history.value.find(entry => entry.requestId === id && entry.apiKeyId === key.id)
    if (!item) return
    // Changing the player cancels only this selection, never the task's delivery.
    const blob = await tasks.getContent(item, refresh)
    if (disposed || controller !== contentController || activeHistoryId.value !== id) return
    videoObjectUrl.value = URL.createObjectURL(blob)
  } catch (error) {
    if (!disposed && controller === contentController && (error as Error).name !== 'AbortError') {
      contentError.value = errorText(error, t('videoStudio.contentUnavailable'))
    }
  } finally {
    if (contentController === controller) {
      loadingContent.value = false
      contentController = null
    }
  }
}

// Only the selected result may load the player; background completion never changes it.
watch([activeHistoryId, viewState], ([id, state]) => {
  clearVideoContent()
  if (!id || state !== 'completed') return
  const key = apiKeys.value.find(entry => entry.id === activeHistoryItem.value?.apiKeyId)
  if (key) void loadVideoContent(key, id)
  else contentError.value = t('videoStudio.historyKeyMissing')
}, { flush: 'sync' })

function handleVideoCanPlay() {
  videoReady.value = true
  videoBuffering.value = false
}

function handleVideoPlaying() {
  videoReady.value = true
  videoStarted.value = true
  videoBuffering.value = false
  videoPlayPending.value = false
}

function handleVideoWaiting() {
  if (videoStarted.value) videoBuffering.value = true
}

async function playVideo() {
  const player = videoPlayer.value
  if (!player || !videoReady.value || videoPlayPending.value) return
  videoPlayPending.value = true
  try {
    await player.play()
  } catch (error) {
    appStore.showError(errorText(error, t('videoStudio.playbackStartFailed')))
  } finally {
    videoPlayPending.value = false
  }
}

function handleVideoPlaybackError() {
  contentError.value = t('videoStudio.playbackFailed')
  releaseVideoObjectURL()
}

async function reloadVideoContent() {
  const item = activeHistoryItem.value
  const key = apiKeys.value.find(entry => entry.id === item?.apiKeyId)
  if (!key || !item) return
  await loadVideoContent(key, item.requestId, true)
}

function errorText(error: unknown, fallback: string): string {
  const value = error as VideoStudioError
  const message = value?.message?.trim() || fallback
  const requestSuffix = value?.requestId ? ` · Request ID: ${value.requestId}` : ''
  return message + requestSuffix
}

async function downloadVideo() {
  const item = activeHistoryItem.value
  const key = apiKeys.value.find(entry => entry.id === item?.apiKeyId)
  if (!key || !item) return
  if (!videoObjectUrl.value) await loadVideoContent(key, item.requestId)
  if (!videoObjectUrl.value || activeHistoryId.value !== item.requestId) return
  const anchor = document.createElement('a')
  anchor.href = videoObjectUrl.value
  anchor.download = 'video-studio-' + item.requestId + '.mp4'
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
}

function selectHistoryItem(item: VideoHistoryItem) {
  if (clearingHistory.value) return
  activeHistoryId.value = item.requestId
}

async function reuseParameters() {
  const item = activeHistoryItem.value
  if (!item || isWorking.value || loadingModels.value) return
  const key = apiKeys.value.find(entry => entry.id === item.apiKeyId)
  if (!key) {
    appStore.showError(t('videoStudio.historyKeyMissing'))
    return
  }
  clearSourceImage()
  selectedKeyId.value = key.id
  operation.value = item.operation
  prompt.value = item.request.prompt
  duration.value = item.request.duration
  resolution.value = item.request.resolution
  aspectRatio.value = normalizeVideoAspectRatio(item.request.aspect_ratio)
  try { localStorage.setItem(lastKeyStorageKey(), String(key.id)) } catch { /* Optional preference. */ }
  await loadModels(item.request.model)
  if (item.operation === 'image') appStore.showSuccess(t('videoStudio.reuploadSource'))
}

async function clearHistory() {
  if (isWorking.value || runningCount.value || deliveringCount.value || loadingHistory.value) return
  if (!window.confirm(t('videoStudio.clearHistoryConfirm'))) return
  try { await tasks.clearHistory() } catch (error) {
    appStore.showError(errorText(error, t('videoStudio.historyClearFailed')))
  }
}

function operationLabel(value: VideoStudioOperation): string {
  return t(value === 'image' ? 'videoStudio.imageShort' : 'videoStudio.textShort')
}

function historyStatusLabel(status: StoredVideoHistoryItem['status']): string {
  return t(`videoStudio.historyStatus.${status}`)
}

function historyStatusClass(status: StoredVideoHistoryItem['status']): string {
  if (status === 'completed') return 'text-emerald-600 dark:text-emerald-400'
  if (status === 'failed') return 'text-red-600 dark:text-red-400'
  return 'text-amber-600 dark:text-amber-400'
}

function historyDotClass(status: StoredVideoHistoryItem['status']): string {
  if (status === 'completed') return 'bg-emerald-500'
  if (status === 'failed') return 'bg-red-500'
  return 'animate-pulse bg-amber-500'
}

function formatElapsed(seconds: number | null): string {
  if (seconds == null) return '—'
  const minutes = Math.floor(seconds / 60)
  return String(minutes).padStart(2, '0') + ':' + String(seconds % 60).padStart(2, '0')
}

onMounted(async () => {
  elapsedTimer = setInterval(() => { now.value = Date.now() }, 1000)
  try {
    const [keys, rates] = await Promise.all([
      listEligibleVideoKeys(),
      userGroupsAPI.getUserGroupRates().catch(() => ({})),
    ])
    if (disposed) return
    apiKeys.value = keys
    userRates.value = rates
    let lastKeyId = 0
    try { lastKeyId = Number(localStorage.getItem(lastKeyStorageKey())) } catch { /* Optional preference. */ }
    selectedKeyId.value = keys.find(key => key.id === lastKeyId)?.id || keys[0]?.id || null
    const draft = consumeVideoStudioImageDraft()
    if (draft) setSourceImageURL(draft.imageUrl)
    await Promise.all([loadModels(), tasks.restore()])
  } catch (error) {
    appStore.showError(errorText(error, t('videoStudio.keysFailed')))
  } finally {
    loadingKeys.value = false
  }
})

onBeforeUnmount(() => {
  disposed = true
  modelController?.abort()
  contentController?.abort()
  if (elapsedTimer) clearInterval(elapsedTimer)
  releaseSourcePreview()
  releaseVideoObjectURL()
})
</script>

<style scoped>
.video-canvas {
  height: clamp(420px, 70vh, 600px);
  height: clamp(420px, 70svh, 600px);
  flex: none;
}

.video-settings-panel :deep(.select-trigger) {
  min-height: 40px;
  border-radius: 8px;
  padding: 8px 12px;
}

.video-settings-panel :deep(textarea.input) {
  border-radius: 8px;
  padding: 9px 12px;
}

.video-settings-panel :deep(.input-label) {
  margin-bottom: 4px;
  font-size: 0.875rem;
}

.video-history-list {
  max-height: 672px;
}

@keyframes video-progress-slide {
  from { transform: translateX(-100%); }
  to { transform: translateX(350%); }
}

.video-indeterminate-progress {
  width: 30%;
  animation: video-progress-slide 1.6s ease-in-out infinite;
}

@media (min-width: 1280px) {
  .video-canvas {
    height: max(600px, calc(100vh - 10rem));
  }

  .video-workspace {
    min-height: calc(100vh - 9rem);
  }

  .video-history-panel,
  .video-history-panel > div {
    min-height: 600px;
  }

  .video-history-list {
    height: 100%;
    max-height: calc(100vh - 11rem);
  }
}

@media (max-width: 1279px) {
  .video-history-list {
    max-height: 360px;
  }
}
</style>
