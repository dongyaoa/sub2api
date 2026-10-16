<template>
  <AppLayout>
    <div class="mx-auto w-full max-w-[1920px]">
      <MediaStudioHeader active="image" />

      <div class="studio-workspace grid grid-cols-1 items-stretch gap-4 xl:grid-cols-[380px_minmax(0,1fr)] 2xl:grid-cols-[380px_minmax(0,1fr)_260px]">
        <aside class="flex min-w-0 flex-col">
          <div class="mb-2 flex min-h-8 items-center justify-between gap-3">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('imageStudio.settings') }}</h2>
            <span
              v-if="estimatedCost != null"
              class="rounded-md bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300"
            >
              {{ t('imageStudio.estimate', { amount: formatUSD(estimatedCost) }) }}
            </span>
          </div>

          <section class="studio-settings-panel image-studio-panel flex-1 rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900">
            <div class="mb-3">
            <div class="grid grid-cols-2 gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-800" role="tablist" :aria-label="t('imageStudio.operationMode')">
            <button
              type="button"
              class="flex h-8 items-center justify-center gap-1.5 rounded-md px-2 text-xs font-medium transition-colors"
              :class="operation === 'generate' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
              :disabled="isWorking"
              :aria-selected="operation === 'generate'"
              role="tab"
              @click="switchOperation('generate')"
            >
              <Icon name="sparkles" size="xs" />
              {{ t('imageStudio.generateMode') }}
            </button>
            <button
              type="button"
              class="flex h-8 items-center justify-center gap-1.5 rounded-md px-2 text-xs font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-45"
              :class="operation === 'edit' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
              :disabled="isWorking || !supportsImageEditing(platform)"
              :aria-selected="operation === 'edit'"
              :title="t('imageStudio.editMode')"
              role="tab"
              @click="switchOperation('edit')"
            >
              <Icon name="edit" size="xs" />
              {{ t('imageStudio.editMode') }}
            </button>
            </div>
          </div>

          <form class="space-y-3" @submit.prevent="generateImages">

              <div v-if="loadingKeys" class="flex h-10 items-center justify-center rounded-lg border border-gray-200 text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <Icon name="refresh" size="sm" class="mr-2 animate-spin" />
                {{ t('imageStudio.loadingKeys') }}
              </div>

              <div v-else-if="apiKeys.length === 0" class="flex min-h-12 items-center justify-between gap-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 dark:border-amber-900/60 dark:bg-amber-950/20">
                <span class="min-w-0 text-xs leading-5 text-amber-800 dark:text-amber-300">{{ t('imageStudio.noKeys') }}</span>
                <RouterLink to="/keys" class="inline-flex shrink-0 items-center gap-1 text-xs font-semibold text-primary-600 hover:text-primary-700 dark:text-primary-400">
                  <Icon name="plus" size="xs" />
                  {{ t('imageStudio.createKey') }}
                </RouterLink>
              </div>

              <div v-else>
                <label class="input-label" for="image-studio-key">{{ t('imageStudio.apiKey') }}</label>
                <Select
                  id="image-studio-key"
                  v-model="selectedKeyId"
                  :options="keyOptions"
                  :disabled="isWorking"
                  :placeholder="t('imageStudio.selectKey')"
                  @change="handleKeySelection"
                >
                  <template #selected="{ option }">
                    <span v-if="option" class="flex min-w-0 items-center gap-2">
                      <span class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase" :class="platformBadgeClass(option.platform)">{{ option.platform }}</span>
                      <span class="truncate">{{ option.label }}</span>
                    </span>
                  </template>
                  <template #option="{ option }">
                    <div class="min-w-0 flex-1">
                      <div class="truncate font-medium text-gray-900 dark:text-white">{{ option.keyName }}</div>
                      <div class="mt-0.5 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                        <span class="truncate">{{ option.groupName }}</span>
                        <span class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase" :class="platformBadgeClass(option.platform)">{{ option.platform }}</span>
                      </div>
                    </div>
                    <Icon v-if="Number(option.value) === selectedKeyId" name="check" size="sm" class="text-primary-500" />
                  </template>
                </Select>
              </div>


              <div>
                <div class="mb-1 flex items-center justify-between gap-2">
                  <label class="text-sm font-medium text-gray-700 dark:text-gray-300" for="image-studio-model">{{ t('imageStudio.model') }}</label>
                  <span v-if="loadingModels" class="flex items-center gap-1 text-xs text-gray-400">
                    <Icon name="refresh" size="xs" class="animate-spin" />
                    {{ t('imageStudio.loading') }}
                  </span>
                </div>
                <Select
                  id="image-studio-model"
                  v-model="model"
                  :options="modelOptions"
                  :disabled="isWorking || loadingModels || !selectedKey"
                  :placeholder="t('imageStudio.selectModel')"
                  :empty-text="t('imageStudio.noModels')"
                />
              </div>
            <div v-if="operation === 'edit'">
              <div class="mb-1 flex items-center justify-between gap-2">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('imageStudio.sourceImage') }}</span>
                <span class="text-[11px] text-gray-400">{{ t('imageStudio.sourceImageLimit', { max: maxSourceImages }) }}</span>
              </div>
              <input
                ref="sourceInput"
                type="file"
                class="sr-only"
                accept=".png,.jpg,.jpeg,.webp,image/png,image/jpeg,image/webp"
                :multiple="maxSourceImages > 1"
                :disabled="isWorking"
                @change="handleSourceInput"
              />
              <div
                v-if="sourceImageFiles.length > 0"
                class="space-y-2 rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-700 dark:bg-dark-800"
                :class="sourceDragActive ? 'border-primary-500 bg-primary-50 dark:bg-primary-950/20' : ''"
                @dragenter.prevent="sourceDragActive = true"
                @dragover.prevent="sourceDragActive = true"
                @dragleave.prevent="sourceDragActive = false"
                @drop.prevent="handleSourceDrop"
              >
                <div class="grid max-h-44 grid-cols-2 gap-2 overflow-y-auto">
                  <div
                    v-for="(file, index) in sourceImageFiles"
                    :key="file.name + '-' + file.lastModified + '-' + index"
                    class="flex min-w-0 items-center gap-2 rounded-md border border-gray-200 bg-white p-1.5 dark:border-dark-600 dark:bg-dark-900"
                  >
                    <img :src="sourcePreviewUrls[index]" :alt="t('imageStudio.sourcePreview')" class="h-10 w-10 shrink-0 rounded object-cover" />
                    <span class="min-w-0 flex-1 truncate text-[11px] text-gray-700 dark:text-gray-200" :title="file.name">{{ file.name }}</span>
                    <button
                      type="button"
                      class="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-gray-400 hover:bg-red-50 hover:text-red-600 disabled:opacity-40 dark:hover:bg-red-950/30 dark:hover:text-red-400"
                      :disabled="isWorking"
                      :title="t('imageStudio.removeImage')"
                      :aria-label="t('imageStudio.removeImage')"
                      @click="removeSourceImage(index)"
                    >
                      <Icon name="trash" size="xs" />
                    </button>
                  </div>
                </div>
                <button
                  v-if="sourceImageFiles.length < maxSourceImages"
                  type="button"
                  class="flex h-8 w-full items-center justify-center gap-1.5 rounded-md border border-dashed border-gray-300 bg-white text-xs font-medium text-gray-500 hover:border-primary-400 hover:text-primary-600 disabled:opacity-40 dark:border-dark-600 dark:bg-dark-900 dark:text-gray-400"
                  :disabled="isWorking"
                  :title="t('imageStudio.addSourceImages')"
                  @click="openSourcePicker"
                >
                  <Icon name="upload" size="xs" />
                  {{ t('imageStudio.addSourceImages') }}
                  <span class="text-[11px] text-gray-400">{{ sourceImageFiles.length }}/{{ maxSourceImages }}</span>
                </button>
              </div>
              <button
                v-else
                type="button"
                class="flex h-20 w-full items-center justify-center gap-3 rounded-lg border border-dashed px-3 text-left transition-colors"
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
                  <span class="block text-xs font-semibold">{{ t('imageStudio.uploadSource') }}</span>
                  <span class="mt-0.5 block text-[11px] opacity-75">{{ t('imageStudio.uploadSourceHint') }}</span>
                </span>
              </button>
            </div>

            <div>
              <div class="mb-1 flex items-center justify-between gap-2">
                <label class="text-sm font-medium text-gray-700 dark:text-gray-300" for="image-studio-prompt">{{ promptLabel }}</label>
                <span class="text-xs tabular-nums text-gray-400">{{ prompt.length }}</span>
              </div>
              <textarea
                id="image-studio-prompt"
                v-model="prompt"
                :disabled="isWorking"
                :placeholder="promptPlaceholder"
                rows="4"
                class="input min-h-[92px] resize-none"
              />
            </div>

            <div v-if="platform === 'openai'">
              <span class="input-label">{{ t('imageStudio.quality') }}</span>
              <div class="grid grid-cols-4 gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-800" role="radiogroup" :aria-label="t('imageStudio.quality')">
                <button
                  v-for="option in qualityOptions"
                  :key="option.value"
                  type="button"
                  class="h-8 rounded-md px-1 text-xs font-medium transition-colors"
                  :class="quality === option.value ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
                  :disabled="isWorking"
                  :aria-pressed="quality === option.value"
                  @click="quality = option.value"
                >
                  {{ option.label }}
                </button>
              </div>
            </div>

            <div class="min-w-0">
              <span class="input-label">{{ t('imageStudio.resolution') }}</span>
              <div class="grid grid-cols-3 gap-2">
                <button
                  v-for="tier in allResolutionTiers"
                  :key="tier"
                  type="button"
                  class="h-9 rounded-md border text-xs font-semibold transition-colors disabled:cursor-not-allowed disabled:opacity-40"
                  :class="resolution === tier ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-950/30 dark:text-primary-300' : 'border-gray-200 text-gray-600 hover:border-gray-300 dark:border-dark-600 dark:text-gray-300 dark:hover:border-dark-500'"
                  :disabled="isWorking || !resolutionOptions.includes(tier)"
                  @click="selectResolution(tier)"
                >
                  {{ tier }}
                </button>
              </div>
            </div>

            <div class="min-w-0">
              <span class="input-label">{{ t('imageStudio.aspectRatio') }}</span>
              <div class="grid grid-cols-4 gap-1.5">
                <button
                  v-for="option in ratioOptions"
                  :key="option.value"
                  type="button"
                  class="flex h-9 min-w-0 items-center justify-center rounded-md border px-0.5 text-[11px] font-medium transition-colors"
                  :class="ratio === option.value ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-950/30 dark:text-primary-300' : 'border-gray-200 text-gray-600 hover:border-gray-300 dark:border-dark-600 dark:text-gray-300 dark:hover:border-dark-500'"
                  :disabled="isWorking"
                  @click="selectRatio(option.value)"
                >
                  {{ option.label }}
                </button>
              </div>
            </div>

            <div class="grid grid-cols-[minmax(0,1fr)_132px] items-end gap-4">
              <div class="min-w-0">
                <label class="input-label" for="image-studio-output-size">{{ t('imageStudio.outputSize') }}</label>
                <input
                  id="image-studio-output-size"
                  :value="outputSizeLabel"
                  type="text"
                  readonly
                  class="input cursor-default bg-gray-50 text-gray-600 dark:bg-dark-800 dark:text-gray-300"
                />
              </div>

              <div class="min-w-0">
                <div class="mb-1 flex items-center justify-between gap-2 whitespace-nowrap">
                  <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('imageStudio.quantity') }}</span>
                  <span class="text-[11px] text-gray-400">{{ t('imageStudio.quantityLimit', { max: maxQuantity }) }}</span>
                </div>
                <div class="grid h-10 grid-cols-[2.25rem_minmax(2.5rem,1fr)_2.25rem] overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
                  <button type="button" class="flex h-full items-center justify-center text-lg text-gray-500 hover:bg-gray-50 disabled:opacity-40 dark:text-gray-300 dark:hover:bg-dark-800" :disabled="isWorking || quantity <= 1" :title="t('imageStudio.decrease')" @click="quantity--">-</button>
                  <span class="flex items-center justify-center border-x border-gray-200 text-sm font-semibold tabular-nums text-gray-900 dark:border-dark-600 dark:text-white">{{ quantity }}</span>
                  <button type="button" class="flex h-full items-center justify-center text-gray-500 hover:bg-gray-50 disabled:opacity-40 dark:text-gray-300 dark:hover:bg-dark-800" :disabled="isWorking || quantity >= maxQuantity" :title="t('imageStudio.increase')" @click="quantity++">
                    <Icon name="plus" size="sm" />
                  </button>
                </div>
              </div>
            </div>

            <section v-if="selectedGroup" class="border-y border-gray-200 py-2.5 dark:border-dark-700" :aria-label="t('imageStudio.pricing')">
              <div class="mb-2 flex items-center justify-between gap-2">
                <span class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('imageStudio.pricing') }}</span>
                <span class="max-w-[150px] truncate text-[11px] text-gray-400">{{ selectedGroup.name }}</span>
              </div>
              <p v-if="loadingPricing" class="flex items-center gap-1.5 text-xs text-gray-400">
                <Icon name="refresh" size="xs" class="animate-spin" />
                {{ t('imageStudio.loadingPricing') }}
              </p>
              <div v-else-if="pricingError" class="flex items-center justify-between gap-2 text-xs text-red-600 dark:text-red-400">
                <span>{{ t('imageStudio.pricingFailed') }}</span>
                <button type="button" class="shrink-0 font-medium underline" @click="loadPricing">{{ t('imageStudio.retryPricing') }}</button>
              </div>
              <p v-else-if="modelPricing?.billing_mode === 'token'" class="text-xs leading-5 text-gray-500 dark:text-gray-400">{{ t('imageStudio.tokenPricingHint') }}</p>
              <div v-else class="grid grid-cols-3 gap-1.5">
                <div
                  v-for="price in priceTiers"
                  :key="price.tier"
                  class="min-w-0 rounded-md border px-2 py-1.5"
                  :class="resolution === price.tier ? 'border-primary-400 bg-primary-50/60 dark:bg-primary-950/20' : 'border-gray-200 dark:border-dark-700'"
                >
                  <div class="flex items-center justify-between gap-1">
                    <span class="text-[11px] font-semibold text-gray-700 dark:text-gray-300">{{ price.tier }}</span>
                    <span class="h-1.5 w-1.5 rounded-full" :class="price.configured ? 'bg-emerald-500' : 'bg-gray-300 dark:bg-gray-600'" />
                  </div>
                  <div class="truncate text-sm font-semibold tabular-nums text-gray-950 dark:text-white">{{ formatUSD(price.unitPrice) }}</div>
                </div>
              </div>
            </section>

            <button
              type="submit"
              class="flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-primary-600 px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500/40 focus:ring-offset-2 disabled:cursor-not-allowed disabled:bg-gray-300 dark:disabled:bg-dark-700"
              :disabled="!canGenerate"
            >
              <Icon :name="isWorking ? 'refresh' : (operation === 'edit' ? 'edit' : 'sparkles')" size="md" :class="isWorking ? 'animate-spin' : ''" />
              <span>{{ actionButtonLabel }}</span>
              <span v-if="!isWorking && estimatedCost != null" class="text-white/75">{{ formatUSD(estimatedCost) }}</span>
            </button>
            <p class="text-center text-[11px] leading-5 text-gray-400">{{ t('imageStudio.multitaskHint') }}</p>
          </form>
          </section>
        </aside>

        <main class="flex min-w-0 flex-col">
          <div class="mb-2 flex min-h-8 flex-wrap items-center justify-between gap-3">
            <div class="flex min-w-0 items-center gap-3">
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('imageStudio.canvas') }}</h2>
              <span v-if="runningCount" class="rounded-full bg-amber-50 px-2 py-1 text-xs text-amber-700 dark:bg-amber-950/40 dark:text-amber-300">{{ t('imageStudio.runningTasks', { count: runningCount }) }}</span>
            </div>
            <div v-if="viewState !== 'idle'" class="flex items-center gap-2 text-sm">
              <span class="h-2 w-2 rounded-full" :class="statusDotClass" />
              <span class="font-medium text-gray-700 dark:text-gray-300">{{ statusLabel }}</span>
              <span v-if="elapsedSeconds !== null" class="text-xs tabular-nums text-gray-400">{{ t('imageStudio.duration') }} {{ formatElapsed(elapsedSeconds) }}</span>
            </div>
            <button type="button" data-testid="new-task" class="ml-auto inline-flex h-7 shrink-0 items-center justify-center gap-1 whitespace-nowrap rounded-md border border-emerald-200 bg-emerald-50 px-2.5 text-xs font-medium text-emerald-700 transition-colors hover:border-emerald-300 hover:bg-emerald-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500/40 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-40 dark:border-emerald-800/70 dark:bg-emerald-950/40 dark:text-emerald-300 dark:hover:bg-emerald-900/50 dark:focus-visible:ring-offset-dark-900" :disabled="isWorking" @click="newTask">
              <Icon name="plus" size="xs" />{{ t('imageStudio.newTask') }}
            </button>
          </div>

          <section class="studio-canvas relative min-h-0 min-w-0 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
            <div v-if="viewState === 'idle'" class="flex h-full min-h-0 flex-col items-center justify-center overflow-y-auto px-6 text-center">
              <div class="mb-4 flex h-14 w-14 items-center justify-center rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800">
                <Icon :name="operation === 'edit' ? 'edit' : 'sparkles'" size="xl" class="text-gray-400 dark:text-gray-500" />
              </div>
              <p class="text-base font-medium text-gray-800 dark:text-gray-200">{{ emptyStateTitle }}</p>
              <p class="mt-1 text-sm text-gray-400">{{ emptyStateHint }}</p>
            </div>

            <div v-else-if="viewState === 'processing'" class="flex h-full min-h-0 min-w-0 flex-col gap-3 overflow-y-auto p-3">
              <p v-if="activeHistoryItem?.pollWarning" class="px-2 text-xs text-amber-600">{{ t('imageStudio.reconnecting') }}</p>
              <div class="grid min-h-0 min-w-0 flex-1 grid-cols-1 gap-3" :class="displayQuantity > 1 ? 'sm:grid-cols-2' : ''">
                <div
                  v-for="index in displayQuantity"
                  :key="index"
                  class="studio-generating-frame relative flex min-h-[160px] min-w-0 items-center justify-center overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800"
                >
                  <div class="relative z-10 flex min-w-0 flex-col items-center px-4 text-center">
                    <span class="studio-spinner mb-4 flex h-12 w-12 shrink-0 items-center justify-center rounded-lg border border-primary-200 bg-white text-primary-600 dark:border-primary-900 dark:bg-dark-900 dark:text-primary-400">
                      <Icon :name="statusOperation === 'edit' ? 'edit' : 'sparkles'" size="lg" />
                    </span>
                    <span class="max-w-full text-sm font-medium text-gray-700 dark:text-gray-200">{{ processingItemLabel(index) }}</span>
                    <span class="mt-1 text-xs text-gray-400">{{ statusLabel }}</span>
                  </div>
                </div>
              </div>
            </div>
            <div v-else-if="viewState === 'failed'" class="flex h-full min-h-0 flex-col items-center justify-center overflow-y-auto px-6 text-center">
              <div class="mb-4 flex h-14 w-14 items-center justify-center rounded-lg bg-red-50 text-red-600 dark:bg-red-950/30 dark:text-red-400">
                <Icon name="exclamationCircle" size="xl" />
              </div>
              <p class="text-base font-semibold text-gray-900 dark:text-white">{{ failedStateTitle }}</p>
              <p class="mt-2 max-w-xl text-sm leading-6 text-gray-500 dark:text-gray-400">{{ errorMessage }}</p>
              <button v-if="activeHistoryItem" type="button" :disabled="isWorking" class="mt-5 inline-flex items-center gap-2 rounded-lg border border-gray-200 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-800" @click="reuseParameters(activeHistoryItem)">
                <Icon name="refresh" size="sm" />
                {{ t('imageStudio.reuseParameters') }}
              </button>
            </div>

            <div v-else class="flex h-full min-h-0 min-w-0 p-3">
              <div v-if="activeImage" class="flex min-h-0 min-w-0 flex-1 flex-col gap-3">
                <figure class="group flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800">
                  <div class="studio-single-media relative flex min-h-0 min-w-0 flex-1 items-center justify-center overflow-hidden bg-gray-100 dark:bg-dark-950">
                    <img
                      v-if="!imageLoadErrors[activeImageIndex]"
                      :src="imageSource(activeImage.url, activeImageIndex)"
                      :alt="t('imageStudio.imageAlt', { index: activeImageIndex + 1 })"
                      class="absolute inset-0 h-full w-full object-contain"
                      loading="eager"
                      @error="markImageError(activeImageIndex)"
                    />
                    <div v-else class="flex flex-col items-center text-center text-gray-500 dark:text-gray-400">
                      <Icon name="exclamationTriangle" size="lg" class="mb-2" />
                      <span class="text-sm">{{ t('imageStudio.imageLoadFailed') }}</span>
                      <button type="button" class="mt-3 inline-flex items-center gap-1 rounded-md border border-gray-200 px-3 py-1.5 text-xs font-medium hover:bg-white dark:border-dark-600 dark:hover:bg-dark-800" @click="reloadImage(activeImageIndex)">
                        <Icon name="refresh" size="xs" />
                        {{ t('imageStudio.reload') }}
                      </button>
                    </div>

                    <button
                      v-if="images.length > 1"
                      type="button"
                      class="absolute left-3 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-md bg-black/65 text-white transition-colors hover:bg-black/80"
                      :title="t('imageStudio.previousImage')"
                      :aria-label="t('imageStudio.previousImage')"
                      @click="showPreviousImage"
                    >
                      <Icon name="chevronLeft" size="md" />
                    </button>
                    <button
                      v-if="images.length > 1"
                      type="button"
                      class="absolute right-3 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-md bg-black/65 text-white transition-colors hover:bg-black/80"
                      :title="t('imageStudio.nextImage')"
                      :aria-label="t('imageStudio.nextImage')"
                      @click="showNextImage"
                    >
                      <Icon name="chevronRight" size="md" />
                    </button>

                    <div v-if="!imageLoadErrors[activeImageIndex]" class="absolute right-3 top-3 flex gap-2 opacity-100 transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100">
                      <button type="button" class="flex h-9 w-9 items-center justify-center rounded-md bg-black/65 text-white hover:bg-black/80" :title="t('imageStudio.createVideo')" @click="createVideoFromImage(activeImage.url)">
                        <Icon name="play" size="sm" />
                      </button>
                      <button type="button" class="flex h-9 w-9 items-center justify-center rounded-md bg-black/65 text-white hover:bg-black/80" :title="t('imageStudio.preview')" @click="openPreview(activeImageIndex)">
                        <Icon name="eye" size="sm" />
                      </button>
                      <button type="button" class="flex h-9 w-9 items-center justify-center rounded-md bg-black/65 text-white hover:bg-black/80" :title="t('imageStudio.download')" @click="downloadImage(activeImage.url, activeImageIndex)">
                        <Icon name="download" size="sm" />
                      </button>
                    </div>
                  </div>
                  <figcaption class="flex min-h-11 shrink-0 items-center justify-between gap-3 px-3 py-2">
                    <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('imageStudio.imagePosition', { current: activeImageIndex + 1, total: images.length }) }}</span>
                    <span class="text-xs text-gray-400">{{ displayResolution }} · {{ displayRatio }}</span>
                  </figcaption>
                </figure>

                <div v-if="activeHistoryItem" class="flex shrink-0 flex-wrap items-center justify-between gap-2 text-xs">
                  <span class="min-w-0 truncate text-gray-500 dark:text-gray-400" :title="activeHistoryItem.model + ' · ' + activeHistoryItem.groupName">{{ activeHistoryItem.model }} · {{ activeHistoryItem.groupName }}</span>
                  <div class="flex shrink-0 gap-2">
                    <button type="button" class="rounded-md border border-gray-200 px-2.5 py-1.5 text-gray-600 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-300 dark:hover:bg-dark-800" :disabled="isWorking" @click="reuseParameters(activeHistoryItem, true)">{{ t('imageStudio.reusePrompt') }}</button>
                    <button type="button" class="rounded-md bg-primary-50 px-2.5 py-1.5 font-medium text-primary-700 dark:bg-primary-950/30 dark:text-primary-300" :disabled="isWorking" @click="reuseParameters(activeHistoryItem)">{{ t('imageStudio.reuseParameters') }}</button>
                  </div>
                </div>

                <div
                  v-if="images.length > 1"
                  class="flex h-16 shrink-0 items-center justify-center gap-2 overflow-x-auto"
                  role="tablist"
                  :aria-label="t('imageStudio.resultNavigation')"
                >
                  <button
                    v-for="(image, index) in images"
                    :key="'thumbnail-' + image.url + '-' + index"
                    type="button"
                    class="relative h-14 w-16 shrink-0 overflow-hidden rounded-md border-2 bg-gray-100 transition-colors dark:bg-dark-800"
                    :class="activeImageIndex === index ? 'border-primary-500' : 'border-transparent hover:border-gray-300 dark:hover:border-dark-500'"
                    :title="t('imageStudio.imagePosition', { current: index + 1, total: images.length })"
                    :aria-selected="activeImageIndex === index"
                    role="tab"
                    @click="selectResultImage(index)"
                  >
                    <img :src="imageSource(image.url, index)" :alt="t('imageStudio.imageAlt', { index: index + 1 })" class="h-full w-full object-cover" />
                    <span class="absolute bottom-0.5 right-0.5 flex h-4 min-w-4 items-center justify-center rounded bg-black/70 px-1 text-[10px] font-medium text-white">{{ index + 1 }}</span>
                  </button>
                </div>
              </div>
            </div>
          </section>
        </main>

        <aside class="studio-history flex min-w-0 flex-col xl:col-span-2 2xl:col-span-1">
          <div class="mb-2 flex min-h-8 items-center justify-between gap-2">
            <div class="flex items-center gap-2">
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('imageStudio.history') }}</h2>
              <span class="text-xs tabular-nums text-gray-400">{{ history.length }}</span>
              <span class="text-[11px] text-gray-400">{{ t('imageStudio.historyRetention', { days: historyRetentionDays }) }}</span>
            </div>
            <button
              type="button"
              class="flex h-8 w-8 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-30 dark:hover:bg-dark-800 dark:hover:text-gray-200"
              :disabled="!history.some(item => item.status !== 'processing') || loadingHistory || clearingHistory || !!deletingHistoryId"
              :title="t('imageStudio.clearHistory')"
              :aria-label="t('imageStudio.clearHistory')"
              @click="clearHistory"
            >
              <Icon :name="clearingHistory ? 'refresh' : 'trash'" size="sm" :class="clearingHistory ? 'animate-spin' : ''" />
            </button>
          </div>

          <section class="studio-history-panel flex-1 overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
            <div v-if="loadingHistory && history.length === 0" class="flex min-h-40 items-center justify-center text-gray-400">
              <Icon name="refresh" size="md" class="animate-spin" />
            </div>

            <div v-else-if="history.length === 0" class="flex min-h-40 flex-col items-center justify-center px-4 text-center text-gray-400">
              <Icon name="clock" size="lg" class="mb-2" />
              <p class="text-xs font-medium">{{ t('imageStudio.historyEmpty') }}</p>
              <p class="mt-1 text-[11px]">{{ t('imageStudio.historyHint') }}</p>
            </div>

            <div v-else class="studio-history-list divide-y divide-gray-100 overflow-y-auto dark:divide-dark-700">
              <div
                v-for="item in history"
                :key="item.id"
                class="grid w-full min-w-0 grid-cols-[minmax(0,1fr)_2rem] items-center gap-x-2 gap-y-2.5 overflow-hidden border-l-2 px-3 py-2.5 text-left transition-colors"
                :class="activeHistoryId === item.id ? 'border-l-primary-500 bg-primary-50/60 dark:bg-primary-950/20' : 'border-l-transparent hover:bg-gray-50 dark:hover:bg-dark-800'"
              >
                <div data-testid="history-meta" class="col-span-2 grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2 text-[10px] leading-4">
                  <div data-testid="history-status" class="flex min-w-0 items-center gap-1.5 overflow-hidden" :title="historyStatusLabel(item.status) + ' · ' + t('imageStudio.duration') + ' ' + itemDurationLabel(item)">
                    <span class="h-1.5 w-1.5 shrink-0 rounded-full" :class="historyDotClass(item.status)" />
                    <span class="shrink-0 whitespace-nowrap" :class="historyStatusClass(item.status)">{{ historyStatusLabel(item.status) }}</span>
                    <span class="min-w-0 truncate whitespace-nowrap tabular-nums text-gray-400">{{ t('imageStudio.duration') }} {{ itemDurationLabel(item) }}</span>
                  </div>
                  <time class="justify-self-end whitespace-nowrap tabular-nums text-gray-400" :datetime="new Date(item.createdAt).toISOString()">{{ formatHistoryTime(item.createdAt) }}</time>
                </div>
                <button
                  type="button"
                  class="col-start-1 row-start-2 flex w-full min-w-0 items-center gap-2.5 text-left"
                  :title="item.prompt"
                  @click="selectHistoryItem(item)"
                >
                  <div class="flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-md bg-gray-100 dark:bg-dark-800">
                    <img
                      v-if="item.thumbnail"
                      :src="item.thumbnail"
                      :alt="t('imageStudio.historyThumbnail')"
                      class="h-full w-full object-cover"
                    />
                    <Icon v-else-if="item.status === 'failed'" name="exclamationCircle" size="md" class="text-red-500" />
                    <Icon v-else name="sparkles" size="md" :class="item.status === 'processing' ? 'animate-pulse text-primary-500' : 'text-gray-400'" />
                  </div>
                  <div class="min-w-0 flex-1">
                    <p class="truncate text-xs font-medium text-gray-800 dark:text-gray-200">{{ item.prompt }}</p>
                    <div class="mt-1 truncate text-[11px] text-gray-400">
                      <span>{{ operationLabel(item.operation) }} · {{ item.resolution }} · {{ item.ratio }}</span>
                    </div>
                  </div>
                </button>
                <button
                  type="button"
                  class="col-start-2 row-start-2 flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-red-50 hover:text-red-600 disabled:cursor-not-allowed disabled:opacity-30 dark:hover:bg-red-950/30 dark:hover:text-red-400"
                  :disabled="item.status === 'processing' || !!deletingHistoryId || clearingHistory"
                  :title="t('imageStudio.deleteHistoryItem')"
                  :aria-label="t('imageStudio.deleteHistoryItem')"
                  @click="deleteHistoryItem(item)"
                >
                  <Icon :name="deletingHistoryId === item.id ? 'refresh' : 'trash'" size="sm" :class="deletingHistoryId === item.id ? 'animate-spin' : ''" />
                </button>
              </div>
            </div>
          </section>
        </aside>
      </div>
    </div>

    <BaseDialog :show="previewIndex !== null" :title="t('imageStudio.previewTitle')" width="full" @close="previewIndex = null">
      <div v-if="previewImage && activeHistoryItem" class="grid min-w-0 gap-5 lg:grid-cols-[minmax(0,1fr)_300px]">
        <div class="relative flex min-h-[280px] min-w-0 items-center justify-center overflow-hidden rounded-xl bg-gray-100 p-3 dark:bg-dark-950">
          <img :src="previewImage.url" :alt="activeHistoryItem.prompt || t('imageStudio.previewTitle')" class="max-h-[68vh] w-full object-contain" />
          <div v-if="images.length > 1" class="absolute bottom-3 left-1/2 flex -translate-x-1/2 items-center gap-3 rounded-full bg-black/70 px-3 py-2 text-xs text-white">
            <button type="button" :aria-label="t('imageStudio.previousImage')" @click="previewIndex = ((previewIndex || 0) - 1 + images.length) % images.length"><Icon name="chevronLeft" size="sm" /></button>
            <span>{{ (previewIndex || 0) + 1 }} / {{ images.length }}</span>
            <button type="button" :aria-label="t('imageStudio.nextImage')" @click="previewIndex = ((previewIndex || 0) + 1) % images.length"><Icon name="chevronRight" size="sm" /></button>
          </div>
        </div>
        <aside class="flex max-h-[68vh] min-w-0 flex-col gap-5 overflow-y-auto text-sm">
          <div>
            <span class="inline-block rounded-md px-2 py-1 text-[11px] font-semibold uppercase" :class="platformBadgeClass(activeHistoryItem.platform)">{{ activeHistoryItem.platform }}</span>
            <h3 class="mt-3 break-all font-semibold text-gray-900 dark:text-white">{{ activeHistoryItem.model }}</h3>
            <p class="mt-1 text-xs text-gray-400">{{ t('imageStudio.group') }} · {{ activeHistoryItem.groupName }}</p>
          </div>
          <dl class="grid grid-cols-2 gap-x-3 gap-y-4 rounded-lg bg-gray-50 p-3 text-xs dark:bg-dark-800">
            <div><dt class="text-gray-400">{{ t('imageStudio.generatedAt') }}</dt><dd class="mt-1 text-gray-800 dark:text-gray-200">{{ formatHistoryTime(activeHistoryItem.createdAt) }}</dd></div>
            <div><dt class="text-gray-400">{{ t('imageStudio.duration') }}</dt><dd class="mt-1 tabular-nums text-gray-800 dark:text-gray-200">{{ itemDurationLabel(activeHistoryItem) }}</dd></div>
            <div><dt class="text-gray-400">{{ t('imageStudio.resolution') }}</dt><dd class="mt-1 text-gray-800 dark:text-gray-200">{{ activeHistoryItem.resolution }} · {{ activeHistoryItem.ratio }}</dd></div>
            <div><dt class="text-gray-400">{{ t('imageStudio.operationMode') }}</dt><dd class="mt-1 text-gray-800 dark:text-gray-200">{{ operationLabel(activeHistoryItem.operation) }}</dd></div>
          </dl>
          <div class="min-h-0">
            <h4 class="mb-2 text-xs font-medium text-gray-500">{{ t('imageStudio.prompt') }}</h4>
            <p class="whitespace-pre-wrap break-words text-sm leading-6 text-gray-700 dark:text-gray-300">{{ activeHistoryItem.prompt }}</p>
          </div>
        </aside>
      </div>
      <template #footer>
        <div class="flex flex-wrap justify-end gap-2">
          <button v-if="activeHistoryItem" type="button" class="btn btn-secondary" :disabled="isWorking" @click="reuseParameters(activeHistoryItem, true)">{{ t('imageStudio.reusePrompt') }}</button>
          <button v-if="activeHistoryItem" type="button" class="btn btn-primary" :disabled="isWorking" @click="reuseParameters(activeHistoryItem)">{{ t('imageStudio.reuseParameters') }}</button>
          <button v-if="previewImage" type="button" class="btn btn-secondary" @click="downloadImage(previewImage.url, previewIndex || 0)">
            <Icon name="download" size="sm" />
            {{ t('imageStudio.download') }}
          </button>
        </div>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import MediaStudioHeader from '@/features/media-studio/MediaStudioHeader.vue'
import { saveVideoStudioImageDraft } from '@/features/video-studio/draft'
import { useAppStore } from '@/stores/app'
import type { ApiKey } from '@/types'
import { listEligibleImageKeys } from './access'
import {
  extractTaskImageData,
  getImageModelPricing,
  listImageModels,
} from './api'
import {
  buildGenerateImageRequest,
  filterImageModels,
  getAspectRatioOptions,
  getMaxImageQuantity,
  getMaxSourceImageQuantity,
  getOpenAIImageSize,
  getPreferredImageModel,
  getResolutionOptions,
  isImageStudioPlatform,
  normalizeStudioSelection,
  supportsImageEditing,
} from './capabilities'
import { estimateImageCost, formatUSD, getImagePriceTiers } from './pricing'
import { useImageTasks, historyDuration, formatHistoryTime, type StudioHistoryItem } from './useImageTasks'
import type {
  GenerateImageRequest,
  ImageAspectRatio,
  ImageModel,
  ImageModelPricing,
  ImageQuality,
  ImageResolutionTier,
  ImageStudioError,
  ImageStudioPlatform,
  StudioOperation,
} from './types'

type HistoryStatus = 'processing' | 'completed' | 'failed'

const MAX_SOURCE_IMAGE_BYTES = 20 * 1024 * 1024
const SOURCE_IMAGE_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp'])
const GEMINI_IMAGE_MODEL_NAMES: Record<string, string> = {
  'gemini-3-pro-image': 'Nano banana Pro',
  'gemini-3-pro-image-preview': 'Nano banana Pro',
  'gemini-3.1-flash-image': 'Nano banana 2',
}

const { t } = useI18n()
const router = useRouter()
const appStore = useAppStore()

const operation = ref<StudioOperation>('generate')
const sourceImageFiles = ref<File[]>([])
const sourcePreviewUrls = ref<string[]>([])
const sourceInput = ref<HTMLInputElement | null>(null)
const sourceDragActive = ref(false)
const apiKeys = ref<ApiKey[]>([])
const selectedKeyId = ref<number | null>(null)
const models = ref<ImageModel[]>([])
const modelPricing = ref<ImageModelPricing | null>(null)
const loadingPricing = ref(false)
const pricingError = ref('')
const model = ref('')
const prompt = ref('')
const quality = ref<ImageQuality>('auto')
const resolution = ref<ImageResolutionTier>('1K')
const ratio = ref<ImageAspectRatio>('1:1')
const quantity = ref(1)
const loadingKeys = ref(true)
const loadingModels = ref(false)
const now = ref(Date.now())
const previewIndex = ref<number | null>(null)
const activeImageIndex = ref(0)
const imageLoadErrors = ref<Record<number, boolean>>({})
const imageReloadTokens = ref<Record<number, number>>({})
const {
  history, activeHistoryId, activeHistoryItem, runningCount, historyRetentionDays,
  loadingHistory, submitting, deletingHistoryId, clearingHistory,
  loadHistory, restore, submit, remove,
} = useImageTasks(apiKeys, message => appStore.showError(message || t('imageStudio.historyLoadFailed')), () => t('imageStudio.emptyResult'))
const task = computed(() => activeHistoryItem.value?.task || null)
const viewState = computed(() => activeHistoryItem.value?.status || 'idle')
const errorMessage = computed(() => activeHistoryItem.value?.errorMessage || '')
const displayQuantity = computed(() => activeHistoryItem.value?.quantity || 1)
const reusing = ref(false)

let modelController: AbortController | null = null
let pricingController: AbortController | null = null
let elapsedTimer: ReturnType<typeof setInterval> | null = null

const selectedKey = computed(() => apiKeys.value.find((key) => key.id === selectedKeyId.value) || null)
const selectedGroup = computed(() => selectedKey.value?.group || null)
const platform = computed<ImageStudioPlatform>(() => {
  const value = selectedGroup.value?.platform
  return isImageStudioPlatform(value) ? value : 'openai'
})

const keyOptions = computed(() => apiKeys.value.map((key) => ({
  value: key.id,
  label: key.name + ' · ' + (key.group?.name || '-'),
  keyName: key.name,
  groupName: key.group?.name || '-',
  platform: key.group?.platform || '',
})))
const modelOptions = computed(() => models.value.map((item) => ({
  value: item.id,
  label: platform.value === 'grok'
    ? item.id
    : platform.value === 'gemini' && GEMINI_IMAGE_MODEL_NAMES[item.id]
    ? GEMINI_IMAGE_MODEL_NAMES[item.id] + ' (' + item.id + ')'
    : item.display_name && item.display_name !== item.id
      ? item.display_name + ' (' + item.id + ')'
      : item.id,
})))
const qualityOptions = computed<Array<{ value: ImageQuality; label: string }>>(() => [
  { value: 'auto', label: t('imageStudio.qualityAuto') },
  { value: 'low', label: t('imageStudio.qualityLow') },
  { value: 'medium', label: t('imageStudio.qualityMedium') },
  { value: 'high', label: t('imageStudio.qualityHigh') },
])
const allResolutionTiers: ImageResolutionTier[] = ['1K', '2K', '4K']
const maxQuantity = computed(() => getMaxImageQuantity(platform.value))
const maxSourceImages = computed(() => getMaxSourceImageQuantity(platform.value))
const openAI4KEnabled = computed(() => (
  platform.value === 'openai' &&
  modelPricing.value?.openai4k_allowed === true
))
const resolutionOptions = computed(() => getResolutionOptions(platform.value, openAI4KEnabled.value))
const ratioOptions = computed(() => getAspectRatioOptions(platform.value))
const outputSizeLabel = computed(() => platform.value === 'openai'
  ? getOpenAIImageSize(ratio.value, resolution.value).replace('x', ' x ')
  : resolution.value + ' · ' + ratio.value)
const priceTiers = computed(() => getImagePriceTiers(modelPricing.value))
const estimatedCost = computed(() => estimateImageCost(modelPricing.value, resolution.value, quantity.value))
const images = computed(() => task.value ? extractTaskImageData(task.value) : [])
const activeImage = computed(() => images.value[activeImageIndex.value] || images.value[0] || null)
const isWorking = computed(() => submitting.value || reusing.value)
const canGenerate = computed(() => {
  if (isWorking.value || !model.value.trim() || !prompt.value.trim()) return false
  if (loadingModels.value || loadingPricing.value || !modelPricing.value || pricingError.value) return false
  if (operation.value === 'edit' && (!supportsImageEditing(platform.value) || (sourceImageFiles.value.length === 0 || sourceImageFiles.value.length > maxSourceImages.value))) return false
  return !!selectedKey.value
})
const statusOperation = computed<StudioOperation>(() => (
  history.value.find((item) => item.id === activeHistoryId.value)?.operation || operation.value
))
const promptLabel = computed(() => t(operation.value === 'edit' ? 'imageStudio.editPrompt' : 'imageStudio.prompt'))
const promptPlaceholder = computed(() => t(operation.value === 'edit' ? 'imageStudio.editPromptPlaceholder' : 'imageStudio.promptPlaceholder'))
const actionButtonLabel = computed(() => {
  if (isWorking.value) return t(operation.value === 'edit' ? 'imageStudio.editing' : 'imageStudio.generating')
  return t(operation.value === 'edit' ? 'imageStudio.startEdit' : 'imageStudio.generate')
})
const emptyStateTitle = computed(() => t(operation.value === 'edit' ? 'imageStudio.editEmpty' : 'imageStudio.empty'))
const emptyStateHint = computed(() => t(operation.value === 'edit' ? 'imageStudio.editEmptyHint' : 'imageStudio.emptyHint'))
const failedStateTitle = computed(() => t(statusOperation.value === 'edit' ? 'imageStudio.editFailed' : 'imageStudio.failed'))
const elapsedSeconds = computed(() => activeHistoryItem.value ? historyDuration(activeHistoryItem.value, now.value) : null)
const statusLabel = computed(() => {
  const editing = statusOperation.value === 'edit'
  if (activeHistoryItem.value?.id.startsWith('local-') && submitting.value) return t(editing ? 'imageStudio.editSubmitting' : 'imageStudio.submitting')
  if (viewState.value === 'processing') return t(editing ? 'imageStudio.editProcessing' : 'imageStudio.processing')
  if (viewState.value === 'completed') return t(editing ? 'imageStudio.editCompleted' : 'imageStudio.completed')
  return t(editing ? 'imageStudio.editFailed' : 'imageStudio.failed')
})
const statusDotClass = computed(() => {
  if (viewState.value === 'completed') return 'bg-emerald-500'
  if (viewState.value === 'failed') return 'bg-red-500'
  return 'animate-pulse bg-amber-500'
})
const previewImage = computed(() => previewIndex.value == null ? null : images.value[previewIndex.value] || null)
const displayResolution = computed(() => activeHistoryItem.value?.resolution || resolution.value)
const displayRatio = computed(() => activeHistoryItem.value?.ratio || ratio.value)

function resetCanvas() {
  activeHistoryId.value = ''
  previewIndex.value = null
  activeImageIndex.value = 0
  imageLoadErrors.value = {}
  imageReloadTokens.value = {}
}

function newTask() {
  if (isWorking.value) return
  resetCanvas()
  prompt.value = ''
  clearSourceImages()
}

function switchOperation(nextOperation: StudioOperation) {
  if (isWorking.value || operation.value === nextOperation) return
  if (nextOperation === 'edit' && !supportsImageEditing(platform.value)) return
  operation.value = nextOperation
  resetCanvas()
}

function keyPreferenceStorage() {
  return 'image_studio_last_key_v1:' + (apiKeys.value[0]?.user_id || 'local')
}

function rememberKey() {
  try { localStorage.setItem(keyPreferenceStorage(), String(selectedKeyId.value)) } catch { /* Optional preference. */ }
}

function platformBadgeClass(value: unknown): string {
  if (value === 'openai') return 'bg-emerald-100 text-emerald-800 dark:bg-emerald-950 dark:text-emerald-300'
  if (value === 'gemini') return 'bg-blue-100 text-blue-700 dark:bg-blue-950 dark:text-blue-300'
  if (value === 'grok') return 'bg-violet-100 text-violet-700 dark:bg-violet-950 dark:text-violet-300'
  return 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-gray-300'
}

function selectResolution(tier: ImageResolutionTier) {
  if (!resolutionOptions.value.includes(tier)) return
  resolution.value = tier
  if (platform.value === 'openai') {
    if (tier === '1K') ratio.value = '1:1'
    if (tier === '2K' && ratio.value === '1:1') ratio.value = '3:2'
  }
}

function selectRatio(nextRatio: ImageAspectRatio) {
  ratio.value = nextRatio
  if (platform.value === 'openai' && resolution.value !== '4K') {
    resolution.value = nextRatio === '1:1' ? '1K' : '2K'
  }
}

async function handleKeySelection() {
  if (!selectedKey.value || isWorking.value) return
  rememberKey()
  if (operation.value === 'edit' && !supportsImageEditing(platform.value)) operation.value = 'generate'
  const normalized = normalizeStudioSelection(platform.value, ratio.value, resolution.value, openAI4KEnabled.value)
  ratio.value = normalized.ratio
  resolution.value = normalized.resolution
  // Keep the user's uploaded files when changing groups; require explicit removal if over the new limit.
  if (sourceImageFiles.value.length > maxSourceImages.value) {
    appStore.showError(t('imageStudio.sourceImageCountError', { max: maxSourceImages.value }))
  }
  quality.value = 'auto'
  quantity.value = Math.min(quantity.value, maxQuantity.value)
  await Promise.all([loadModels(getPreferredImageModel(platform.value)), loadHistory(selectedKey.value)])
}

function openSourcePicker() {
  if (!isWorking.value) sourceInput.value?.click()
}

function isSupportedSourceImage(file: File): boolean {
  if (SOURCE_IMAGE_TYPES.has(file.type.toLowerCase())) return true
  return /[.](png|jpe?g|webp)$/i.test(file.name)
}

function addSourceImages(files: File[]) {
  sourceDragActive.value = false
  if (isWorking.value || files.length === 0) return

  const remaining = maxSourceImages.value - sourceImageFiles.value.length
  if (remaining <= 0) {
    appStore.showError(t('imageStudio.sourceImageCountError', { max: maxSourceImages.value }))
    return
  }

  const candidates = files.slice(0, remaining)
  for (const file of candidates) {
    if (!isSupportedSourceImage(file)) {
      appStore.showError(t('imageStudio.sourceImageTypeError'))
      continue
    }
    if (file.size > MAX_SOURCE_IMAGE_BYTES) {
      appStore.showError(t('imageStudio.sourceImageSizeError'))
      continue
    }
    sourceImageFiles.value.push(file)
    sourcePreviewUrls.value.push(URL.createObjectURL(file))
  }

  if (files.length > remaining) {
    appStore.showError(t('imageStudio.sourceImageCountError', { max: maxSourceImages.value }))
  }
}

function handleSourceInput(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from((input as HTMLInputElement).files || [])
  addSourceImages(files)
  input.value = ''
}

function handleSourceDrop(event: DragEvent) {
  sourceDragActive.value = false
  if (isWorking.value) return
  addSourceImages(Array.from(event.dataTransfer?.files || []))
}

function removeSourceImage(index: number) {
  const previewUrl = sourcePreviewUrls.value[index]
  if (previewUrl) URL.revokeObjectURL(previewUrl)
  sourceImageFiles.value.splice(index, 1)
  sourcePreviewUrls.value.splice(index, 1)
}

function clearSourceImages() {
  sourcePreviewUrls.value.forEach(url => URL.revokeObjectURL(url))
  sourcePreviewUrls.value = []
  sourceImageFiles.value = []
}

function processingItemLabel(index: number): string {
  const key = statusOperation.value === 'edit' ? 'imageStudio.editRendering' : 'imageStudio.rendering'
  return t(key, { current: index, total: displayQuantity.value })
}

function operationLabel(value: StudioOperation): string {
  return t(value === 'edit' ? 'imageStudio.editShort' : 'imageStudio.generateShort')
}

async function loadModels(preferredModel = '') {
  modelController?.abort()
  loadingModels.value = true
  models.value = []
  model.value = preferredModel
  const key = selectedKey.value
  if (!key) {
    loadingModels.value = false
    return
  }
  const controller = new AbortController()
  modelController = controller
  try {
    const response = await listImageModels(key.key, controller.signal)
    if (modelController !== controller || selectedKey.value?.id !== key.id) return
    const filtered = filterImageModels(platform.value, response.data || [])
    models.value = filtered
    if (!filtered.some((item) => item.id === preferredModel)) {
      model.value = filtered[0]?.id || ''
    }
  } catch (error) {
    if (modelController === controller && selectedKey.value?.id === key.id && (error as Error).name !== 'AbortError') {
      model.value = ''
      appStore.showError(errorText(error, t('imageStudio.modelsFailed')))
    }
  } finally {
    if (modelController === controller) {
      loadingModels.value = false
      modelController = null
    }
  }
}

async function loadPricing() {
  pricingController?.abort()
  pricingController = null
  modelPricing.value = null
  pricingError.value = ''
  loadingPricing.value = false
  const key = selectedKey.value
  const selectedModel = model.value.trim()
  if (!key || !selectedModel || loadingModels.value) return
  const controller = new AbortController()
  pricingController = controller
  loadingPricing.value = true
  try {
    const quote = await getImageModelPricing(key.key, selectedModel, controller.signal)
    if (pricingController !== controller || selectedKey.value?.id !== key.id || model.value.trim() !== selectedModel) return
    modelPricing.value = quote
    const normalized = normalizeStudioSelection(platform.value, ratio.value, resolution.value, openAI4KEnabled.value)
    ratio.value = normalized.ratio
    resolution.value = normalized.resolution
  } catch (error) {
    if (pricingController === controller && (error as Error).name !== 'AbortError') {
      pricingError.value = errorText(error, t('imageStudio.pricingFailed'))
    }
  } finally {
    if (pricingController === controller) {
      loadingPricing.value = false
      pricingController = null
    }
  }
}

watch([selectedKeyId, model, loadingModels], () => { void loadPricing() }, { flush: 'sync' })

function selectHistoryItem(item: StudioHistoryItem) {
  activeHistoryId.value = item.id
  previewIndex.value = null
  activeImageIndex.value = 0
  imageLoadErrors.value = {}
  imageReloadTokens.value = {}
}

async function clearHistory() {
  if (clearingHistory.value) return
  const items = history.value.filter(item => item.status !== 'processing')
  if (!items.length || !window.confirm(t('imageStudio.clearCompletedConfirm'))) return
  clearingHistory.value = true
  try { for (const item of items) await remove(item) }
  catch (error) { appStore.showError(errorText(error, t('imageStudio.historyClearFailed'))) }
  finally { clearingHistory.value = false }
}

async function deleteHistoryItem(item: StudioHistoryItem) {
  if (item.status === 'processing' || deletingHistoryId.value || clearingHistory.value) return
  if (!window.confirm(t('imageStudio.deleteHistoryConfirm'))) return
  deletingHistoryId.value = item.id
  try { await remove(item) }
  catch (error) { appStore.showError(errorText(error, t('imageStudio.historyDeleteFailed'))) }
  finally { deletingHistoryId.value = '' }
}

function itemDurationLabel(item: StudioHistoryItem): string {
  const seconds = historyDuration(item, now.value)
  return seconds == null ? '—' : formatElapsed(seconds)
}

async function reuseParameters(item: StudioHistoryItem, promptOnly = false) {
  if (isWorking.value) return
  if (promptOnly) {
    prompt.value = item.prompt
    previewIndex.value = null
    return
  }
  const key = apiKeys.value.find(key => key.id === item.apiKeyId)
  if (!key) { appStore.showError(t('imageStudio.reuseKeyUnavailable')); return }
  reusing.value = true
  try {
    selectedKeyId.value = key.id
    rememberKey()
    await loadModels(item.model)
    await loadPricing()
    if (model.value !== item.model) {
      appStore.showError(t('imageStudio.reuseModelUnavailable'))
      return
    }
    operation.value = item.operation
    prompt.value = item.prompt
    quality.value = item.request.quality || 'auto'
    quantity.value = Math.min(item.quantity, maxQuantity.value)
    const normalized = normalizeStudioSelection(platform.value, item.ratio, item.resolution, openAI4KEnabled.value)
    ratio.value = normalized.ratio
    resolution.value = normalized.resolution
    if (ratio.value !== item.ratio || resolution.value !== item.resolution) {
      appStore.showError(t('imageStudio.reuseAdjusted'))
    }
    clearSourceImages()
    if (item.operation === 'edit') {
      sourceImageFiles.value = (item.sourceFiles || []).slice(0, maxSourceImages.value)
      sourcePreviewUrls.value = sourceImageFiles.value.map(file => URL.createObjectURL(file))
      if (!sourceImageFiles.value.length) appStore.showError(t('imageStudio.reuploadReferences'))
    }
    previewIndex.value = null
  } finally { reusing.value = false }
}

function historyStatusLabel(status: HistoryStatus): string {
  return t('imageStudio.historyStatus.' + status)
}

function historyStatusClass(status: HistoryStatus): string {
  if (status === 'completed') return 'text-emerald-600 dark:text-emerald-400'
  if (status === 'failed') return 'text-red-600 dark:text-red-400'
  return 'text-amber-600 dark:text-amber-400'
}

function historyDotClass(status: HistoryStatus): string {
  if (status === 'completed') return 'bg-emerald-500'
  if (status === 'failed') return 'bg-red-500'
  return 'animate-pulse bg-amber-500'
}

function currentRequest(): GenerateImageRequest {
  return buildGenerateImageRequest({
    platform: platform.value,
    model: model.value,
    prompt: prompt.value,
    quantity: quantity.value,
    ratio: ratio.value,
    resolution: resolution.value,
    quality: quality.value,
    allowOpenAI4K: openAI4KEnabled.value,
  })
}

async function generateImages() {
  if (!canGenerate.value || !selectedKey.value) return
  rememberKey()
  previewIndex.value = null
  activeImageIndex.value = 0
  imageLoadErrors.value = {}
  imageReloadTokens.value = {}
  await submit(selectedKey.value, {
    request: currentRequest(),
    operation: operation.value,
    platform: platform.value,
    ratio: ratio.value,
    resolution: resolution.value,
    sourceFiles: operation.value === 'edit' ? [...sourceImageFiles.value] : [],
  })
}

function errorText(error: unknown, fallback: string): string {
  const value = error as ImageStudioError
  const message = value?.message || fallback
  const suffix = value?.requestId ? ' (' + value.requestId + ')' : ''
  return message + suffix
}

function formatElapsed(seconds: number): string {
  const minutes = Math.floor(seconds / 60)
  const rest = seconds % 60
  return String(minutes).padStart(2, '0') + ':' + String(rest).padStart(2, '0')
}

function openPreview(index: number) {
  previewIndex.value = index
}

function selectResultImage(index: number) {
  if (index < 0 || index >= images.value.length) return
  activeImageIndex.value = index
}

function showPreviousImage() {
  if (images.value.length <= 1) return
  activeImageIndex.value = (activeImageIndex.value - 1 + images.value.length) % images.value.length
}

function showNextImage() {
  if (images.value.length <= 1) return
  activeImageIndex.value = (activeImageIndex.value + 1) % images.value.length
}

function createVideoFromImage(url: string) {
  if (!saveVideoStudioImageDraft(url)) {
    appStore.showError(t('imageStudio.createVideoFailed'))
    return
  }
  void router.push('/video-studio')
}

function markImageError(index: number) {
  imageLoadErrors.value = { ...imageLoadErrors.value, [index]: true }
}

function reloadImage(index: number) {
  imageReloadTokens.value = { ...imageReloadTokens.value, [index]: Date.now() }
  imageLoadErrors.value = { ...imageLoadErrors.value, [index]: false }
}

function imageSource(url: string, index: number): string {
  const token = imageReloadTokens.value[index]
  if (!token || url.startsWith('data:')) return url
  try {
    const parsed = new URL(url, window.location.origin)
    parsed.searchParams.set('_image_retry', String(token))
    return parsed.toString()
  } catch {
    return url
  }
}

async function downloadImage(url: string, index: number) {
  const filename = 'image-studio-' + Date.now() + '-' + (index + 1) + '.png'
  try {
    const response = await fetch(url)
    if (!response.ok) throw new Error(response.statusText)
    const objectUrl = URL.createObjectURL(await response.blob())
    triggerDownload(objectUrl, filename)
    setTimeout(() => URL.revokeObjectURL(objectUrl), 1000)
  } catch {
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.target = '_blank'
    anchor.rel = 'noopener noreferrer'
    anchor.click()
  }
}

function triggerDownload(url: string, filename: string) {
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
}

onMounted(async () => {
  elapsedTimer = setInterval(() => { now.value = Date.now() }, 1000)
  try {
    apiKeys.value = await listEligibleImageKeys()
    loadingKeys.value = false
    let remembered: number | null = null
    try { remembered = Number(localStorage.getItem(keyPreferenceStorage())) } catch { /* Optional preference. */ }
    const defaultKey = apiKeys.value.find(key => key.id === remembered)
      || apiKeys.value.find(key => key.group?.platform === 'openai') || apiKeys.value[0]
    selectedKeyId.value = defaultKey?.id || null
    await Promise.all([loadModels(getPreferredImageModel(platform.value)), restore()])
  } catch (error) {
    appStore.showError(errorText(error, t('imageStudio.keysFailed')))
  } finally {
    loadingKeys.value = false
  }
})

onBeforeUnmount(() => {
  modelController?.abort()
  pricingController?.abort()
  if (elapsedTimer) clearInterval(elapsedTimer)
  clearSourceImages()
})
</script>

<style scoped>
.image-studio-panel :deep(.select-trigger) {
  min-height: 40px;
  border-radius: 8px;
  padding: 8px 12px;
}

.image-studio-panel :deep(input.input) {
  height: 40px;
  min-height: 40px;
  border-radius: 8px;
  padding-top: 8px;
  padding-bottom: 8px;
}

.image-studio-panel :deep(textarea.input) {
  border-radius: 8px;
  padding: 8px 12px;
}

.image-studio-panel :deep(.input-label) {
  margin-bottom: 4px;
}

.studio-history-list {
  max-height: 560px;
}

.studio-canvas {
  /* Definite height in every state; intrinsic image dimensions must not size the workspace. */
  height: clamp(420px, 70vh, 600px);
  height: clamp(420px, 70svh, 600px);
  flex: none;
}

.studio-single-media {
  flex: 1 1 0%;
}

@media (min-width: 1280px) {
  .studio-canvas {
    height: max(600px, calc(100vh - 10rem));
  }
}

@media (min-width: 1536px) {
  .studio-history-panel {
    height: max(600px, calc(100vh - 10rem));
  }

  .studio-history-panel > div {
    height: 100%;
  }

  .studio-history-list {
    max-height: 100%;
  }
}

.studio-generating-frame::after {
  content: '';
  position: absolute;
  inset: 10%;
  border: 1px solid rgb(209 213 219 / 0.65);
  border-radius: 6px;
  animation: studio-frame-pulse 2s ease-in-out infinite;
}

.studio-spinner {
  animation: studio-spinner-turn 2.4s linear infinite;
}

@keyframes studio-frame-pulse {
  0%, 100% { opacity: 0.35; transform: scale(0.98); }
  50% { opacity: 1; transform: scale(1); }
}

@keyframes studio-spinner-turn {
  to { transform: rotate(360deg); }
}

@media (prefers-reduced-motion: reduce) {
  .studio-generating-frame::after,
  .studio-spinner {
    animation: none;
  }
}
</style>
