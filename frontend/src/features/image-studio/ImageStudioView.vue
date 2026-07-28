<template>
  <AppLayout>
    <div class="mx-auto w-full max-w-[1920px]">
      <header class="mb-3 flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 pb-3 dark:border-dark-700">
        <div class="flex min-w-0 items-center gap-3">
          <h1 class="truncate text-xl font-semibold text-gray-950 dark:text-white">{{ t('imageStudio.title') }}</h1>
          <span class="inline-flex shrink-0 items-center gap-1.5 rounded-md bg-primary-50 px-2 py-1 text-xs font-medium text-primary-700 dark:bg-primary-950/30 dark:text-primary-300">
            <Icon name="sparkles" size="xs" />
            {{ t('imageStudio.eyebrow') }}
          </span>
        </div>
        <div class="flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
          <Icon :name="connectionMode === 'external' ? 'globe' : 'cloud'" size="sm" />
          <span>{{ storageStatusLabel }}</span>
        </div>
      </header>

      <div class="grid grid-cols-1 items-start gap-4 xl:grid-cols-[380px_minmax(0,1fr)] 2xl:grid-cols-[380px_minmax(0,1fr)_260px]">
        <aside class="image-studio-panel rounded-lg border border-gray-200 bg-white p-4 dark:border-dark-700 dark:bg-dark-900">
          <div class="mb-3 flex items-center justify-between gap-3">
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('imageStudio.settings') }}</h2>
            <span
              v-if="connectionMode === 'group' && selectedGroup"
              class="rounded-md bg-emerald-50 px-2 py-1 text-xs font-medium text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300"
            >
              {{ t('imageStudio.estimate', { amount: formatUSD(estimatedCost) }) }}
            </span>
          </div>

          <div class="mb-3 grid grid-cols-2 gap-2">
            <div class="grid grid-cols-2 gap-1 rounded-lg bg-gray-100 p-1 dark:bg-dark-800" role="tablist" :aria-label="t('imageStudio.connectionMode')">
            <button
              type="button"
              class="flex h-8 items-center justify-center gap-1.5 rounded-md px-2 text-xs font-medium transition-colors"
              :class="connectionMode === 'group' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
              :disabled="isWorking"
              :aria-selected="connectionMode === 'group'"
              role="tab"
              @click="switchConnectionMode('group')"
            >
              <Icon name="server" size="xs" />
              {{ t('imageStudio.groupMode') }}
            </button>
            <button
              type="button"
              class="flex h-8 items-center justify-center gap-1.5 rounded-md px-2 text-xs font-medium transition-colors"
              :class="connectionMode === 'external' ? 'bg-white text-gray-900 shadow-sm dark:bg-dark-700 dark:text-white' : 'text-gray-500 hover:text-gray-800 dark:text-gray-400 dark:hover:text-gray-200'"
              :disabled="isWorking"
              :aria-selected="connectionMode === 'external'"
              role="tab"
              @click="switchConnectionMode('external')"
            >
              <Icon name="externalLink" size="xs" />
              {{ t('imageStudio.externalMode') }}
            </button>
          </div>

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
              :disabled="isWorking || platform !== 'openai'"
              :aria-selected="operation === 'edit'"
              :title="platform !== 'openai' ? t('imageStudio.editOpenAIOnly') : t('imageStudio.editMode')"
              role="tab"
              @click="switchOperation('edit')"
            >
              <Icon name="edit" size="xs" />
              {{ t('imageStudio.editMode') }}
            </button>
            </div>
          </div>

          <form class="space-y-3" @submit.prevent="generateImages">
            <template v-if="connectionMode === 'group'">
              <div v-if="loadingKeys" class="flex h-10 items-center justify-center rounded-lg border border-gray-200 text-xs text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <Icon name="refresh" size="sm" class="mr-2 animate-spin" />
                {{ t('imageStudio.loadingKeys') }}
              </div>

              <div v-else-if="apiKeys.length === 0" class="flex h-10 items-center justify-between gap-3 rounded-lg border border-amber-200 bg-amber-50 px-3 dark:border-amber-900/60 dark:bg-amber-950/20">
                <span class="truncate text-xs text-amber-800 dark:text-amber-300">{{ t('imageStudio.noKeys') }}</span>
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
                  <template #option="{ option }">
                    <div class="min-w-0 flex-1">
                      <div class="truncate font-medium text-gray-900 dark:text-white">{{ option.keyName }}</div>
                      <div class="mt-0.5 flex items-center gap-2 text-xs text-gray-500 dark:text-gray-400">
                        <span class="truncate">{{ option.groupName }}</span>
                        <span class="uppercase">{{ option.platform }}</span>
                      </div>
                    </div>
                    <Icon v-if="Number(option.value) === selectedKeyId" name="check" size="sm" class="text-primary-500" />
                  </template>
                </Select>
              </div>

              <div>
                <label class="input-label" for="image-studio-base-url">{{ t('imageStudio.baseUrl') }}</label>
                <div class="relative">
                  <Icon name="lock" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                  <input id="image-studio-base-url" :value="groupBaseUrl" type="text" readonly class="input cursor-not-allowed pl-9 pr-3 text-gray-500 dark:text-gray-400" />
                </div>
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
            </template>

            <template v-else>
              <div>
                <label class="input-label" for="image-studio-external-base-url">{{ t('imageStudio.baseUrl') }}</label>
                <div class="relative">
                  <Icon name="globe" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                  <input
                    id="image-studio-external-base-url"
                    v-model.trim="externalBaseUrl"
                    type="url"
                    class="input pl-9"
                    :disabled="isWorking"
                    :placeholder="t('imageStudio.externalBasePlaceholder')"
                    autocomplete="url"
                    spellcheck="false"
                  />
                </div>
              </div>

              <div>
                <label class="input-label" for="image-studio-external-key">{{ t('imageStudio.externalApiKey') }}</label>
                <div class="relative">
                  <Icon name="key" size="sm" class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
                  <input
                    id="image-studio-external-key"
                    v-model="externalApiKey"
                    :type="externalKeyVisible ? 'text' : 'password'"
                    class="input pl-9 pr-10"
                    :disabled="isWorking"
                    :placeholder="t('imageStudio.externalKeyPlaceholder')"
                    autocomplete="new-password"
                    spellcheck="false"
                  />
                  <button
                    type="button"
                    class="absolute right-1 top-1/2 flex h-8 w-8 -translate-y-1/2 items-center justify-center rounded-md text-gray-400 hover:bg-gray-100 hover:text-gray-600 dark:hover:bg-dark-700 dark:hover:text-gray-200"
                    :title="t(externalKeyVisible ? 'imageStudio.hideKey' : 'imageStudio.showKey')"
                    :aria-label="t(externalKeyVisible ? 'imageStudio.hideKey' : 'imageStudio.showKey')"
                    @click="externalKeyVisible = !externalKeyVisible"
                  >
                    <Icon :name="externalKeyVisible ? 'eyeOff' : 'eye'" size="sm" />
                  </button>
                </div>
              </div>

              <div>
                <label class="input-label" for="image-studio-external-model">{{ t('imageStudio.model') }}</label>
                <input
                  id="image-studio-external-model"
                  v-model.trim="externalModel"
                  type="text"
                  class="input"
                  :disabled="isWorking"
                  :placeholder="t('imageStudio.externalModelPlaceholder')"
                  autocomplete="off"
                  spellcheck="false"
                />
              </div>
            </template>

            <div v-if="operation === 'edit'">
              <div class="mb-1 flex items-center justify-between gap-2">
                <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('imageStudio.sourceImage') }}</span>
                <span class="text-[11px] text-gray-400">{{ t('imageStudio.sourceImageLimit') }}</span>
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
                v-if="sourcePreviewUrl && sourceImageFile"
                class="flex h-20 items-center gap-3 overflow-hidden rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-700 dark:bg-dark-800"
              >
                <img :src="sourcePreviewUrl" :alt="t('imageStudio.sourcePreview')" class="h-16 w-16 shrink-0 rounded-md object-cover" />
                <div class="min-w-0 flex-1">
                  <p class="truncate text-xs font-medium text-gray-800 dark:text-gray-200" :title="sourceImageFile.name">{{ sourceImageFile.name }}</p>
                  <p class="mt-1 text-[11px] text-gray-400">{{ formatFileSize(sourceImageFile.size) }}</p>
                </div>
                <div class="flex shrink-0 gap-1">
                  <button type="button" class="flex h-8 w-8 items-center justify-center rounded-md text-gray-500 hover:bg-white hover:text-primary-600 disabled:opacity-40 dark:hover:bg-dark-700 dark:hover:text-primary-400" :disabled="isWorking" :title="t('imageStudio.replaceImage')" @click="openSourcePicker">
                    <Icon name="upload" size="sm" />
                  </button>
                  <button type="button" class="flex h-8 w-8 items-center justify-center rounded-md text-gray-500 hover:bg-white hover:text-red-600 disabled:opacity-40 dark:hover:bg-dark-700 dark:hover:text-red-400" :disabled="isWorking" :title="t('imageStudio.removeImage')" @click="clearSourceImage">
                    <Icon name="trash" size="sm" />
                  </button>
                </div>
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
                rows="3"
                class="input min-h-[76px] resize-none"
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

            <div class="grid grid-cols-2 gap-3">
              <div class="min-w-0">
                <span class="input-label">{{ t('imageStudio.resolution') }}</span>
                <div class="grid grid-cols-3 gap-1.5">
                  <button
                    v-for="tier in allResolutionTiers"
                    :key="tier"
                    type="button"
                    class="h-8 rounded-md border text-xs font-semibold transition-colors"
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
                    class="flex h-8 items-center justify-center rounded-md border px-0.5 text-[11px] font-medium transition-colors"
                    :class="ratio === option.value ? 'border-primary-500 bg-primary-50 text-primary-700 dark:bg-primary-950/30 dark:text-primary-300' : 'border-gray-200 text-gray-600 hover:border-gray-300 dark:border-dark-600 dark:text-gray-300 dark:hover:border-dark-500'"
                    :disabled="isWorking"
                    @click="selectRatio(option.value)"
                  >
                    {{ option.label }}
                  </button>
                </div>
              </div>
            </div>

            <div class="grid grid-cols-2 gap-3">
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
                <div class="mb-1 flex items-center justify-between gap-1">
                  <span class="text-sm font-medium text-gray-700 dark:text-gray-300">{{ t('imageStudio.quantity') }}</span>
                  <span class="text-[11px] text-gray-400">{{ t('imageStudio.quantityLimit') }}</span>
                </div>
                <div class="grid h-10 grid-cols-[1fr_2.5rem_1fr] overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
                  <button type="button" class="flex h-full items-center justify-center text-lg text-gray-500 hover:bg-gray-50 disabled:opacity-40 dark:text-gray-300 dark:hover:bg-dark-800" :disabled="isWorking || quantity <= 1" :title="t('imageStudio.decrease')" @click="quantity--">-</button>
                  <span class="flex items-center justify-center border-x border-gray-200 text-sm font-semibold tabular-nums text-gray-900 dark:border-dark-600 dark:text-white">{{ quantity }}</span>
                  <button type="button" class="flex h-full items-center justify-center text-gray-500 hover:bg-gray-50 disabled:opacity-40 dark:text-gray-300 dark:hover:bg-dark-800" :disabled="isWorking || quantity >= 4" :title="t('imageStudio.increase')" @click="quantity++">
                    <Icon name="plus" size="sm" />
                  </button>
                </div>
              </div>
            </div>

            <section v-if="connectionMode === 'group' && selectedGroup" class="border-y border-gray-200 py-2.5 dark:border-dark-700" :aria-label="t('imageStudio.pricing')">
              <div class="mb-2 flex items-center justify-between gap-2">
                <span class="text-xs font-medium text-gray-700 dark:text-gray-300">{{ t('imageStudio.pricing') }}</span>
                <span class="max-w-[150px] truncate text-[11px] text-gray-400">{{ selectedGroup.name }}</span>
              </div>
              <div class="grid grid-cols-3 gap-1.5">
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

            <div v-else-if="connectionMode === 'external'" class="flex items-start gap-2 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-[11px] leading-4 text-amber-800 dark:border-amber-900/60 dark:bg-amber-950/20 dark:text-amber-300">
              <Icon name="exclamationTriangle" size="xs" class="mt-0.5 shrink-0" />
              <span>{{ t('imageStudio.externalNoPricing') }}</span>
            </div>

            <button
              type="submit"
              class="flex h-11 w-full items-center justify-center gap-2 rounded-lg bg-primary-600 px-4 text-sm font-semibold text-white transition-colors hover:bg-primary-700 focus:outline-none focus:ring-2 focus:ring-primary-500/40 focus:ring-offset-2 disabled:cursor-not-allowed disabled:bg-gray-300 dark:disabled:bg-dark-700"
              :disabled="!canGenerate"
            >
              <Icon :name="isWorking ? 'refresh' : (operation === 'edit' ? 'edit' : 'sparkles')" size="md" :class="isWorking ? 'animate-spin' : ''" />
              <span>{{ actionButtonLabel }}</span>
              <span v-if="!isWorking && connectionMode === 'group' && selectedGroup" class="text-white/75">{{ formatUSD(estimatedCost) }}</span>
            </button>
          </form>
        </aside>

        <main class="min-w-0">
          <div class="mb-2 flex min-h-8 flex-wrap items-center justify-between gap-3">
            <div class="flex min-w-0 items-center gap-3">
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('imageStudio.canvas') }}</h2>
              <p v-if="taskId" class="max-w-[440px] truncate text-xs text-gray-400" :title="taskId">{{ taskId }}</p>
            </div>
            <div v-if="viewState !== 'idle'" class="flex items-center gap-2 text-sm">
              <span class="h-2 w-2 rounded-full" :class="statusDotClass" />
              <span class="font-medium text-gray-700 dark:text-gray-300">{{ statusLabel }}</span>
              <span v-if="isWorking" class="tabular-nums text-gray-400">{{ formatElapsed(elapsedSeconds) }}</span>
            </div>
          </div>

          <section class="studio-canvas relative min-h-[600px] overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
            <div v-if="viewState === 'idle'" class="flex min-h-[600px] flex-col items-center justify-center px-6 text-center">
              <div class="mb-4 flex h-14 w-14 items-center justify-center rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800">
                <Icon :name="operation === 'edit' ? 'edit' : 'sparkles'" size="xl" class="text-gray-400 dark:text-gray-500" />
              </div>
              <p class="text-base font-medium text-gray-800 dark:text-gray-200">{{ emptyStateTitle }}</p>
              <p class="mt-1 text-sm text-gray-400">{{ emptyStateHint }}</p>
            </div>

            <div v-else-if="isWorking" class="flex p-3">
              <div class="w-full gap-3" :class="quantity > 1 ? 'grid grid-cols-1 md:grid-cols-2' : 'flex'">
                <div
                  v-for="index in quantity"
                  :key="index"
                  class="studio-generating-frame relative flex items-center justify-center overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800"
                  :class="quantity === 1 ? 'min-h-[520px] flex-1' : 'min-h-[280px]'"
                  :style="quantity > 1 ? { aspectRatio: previewAspectRatio } : undefined"
                >
                  <div class="relative z-10 flex flex-col items-center px-4 text-center">
                    <span class="studio-spinner mb-4 flex h-12 w-12 items-center justify-center rounded-lg border border-primary-200 bg-white text-primary-600 dark:border-primary-900 dark:bg-dark-900 dark:text-primary-400">
                      <Icon :name="operation === 'edit' ? 'edit' : 'sparkles'" size="lg" />
                    </span>
                    <span class="text-sm font-medium text-gray-700 dark:text-gray-200">{{ processingItemLabel(index) }}</span>
                    <span class="mt-1 text-xs text-gray-400">{{ statusLabel }}</span>
                  </div>
                </div>
              </div>
            </div>

            <div v-else-if="viewState === 'failed'" class="flex min-h-[600px] flex-col items-center justify-center px-6 text-center">
              <div class="mb-4 flex h-14 w-14 items-center justify-center rounded-lg bg-red-50 text-red-600 dark:bg-red-950/30 dark:text-red-400">
                <Icon name="exclamationCircle" size="xl" />
              </div>
              <p class="text-base font-semibold text-gray-900 dark:text-white">{{ failedStateTitle }}</p>
              <p class="mt-2 max-w-xl text-sm leading-6 text-gray-500 dark:text-gray-400">{{ errorMessage }}</p>
              <button type="button" class="mt-5 inline-flex items-center gap-2 rounded-lg border border-gray-200 px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-800" @click="generateImages">
                <Icon name="refresh" size="sm" />
                {{ t('imageStudio.retry') }}
              </button>
            </div>

            <div v-else class="flex p-3">
              <div class="w-full gap-3" :class="images.length > 1 ? 'grid grid-cols-1 md:grid-cols-2' : 'flex min-w-0'">
                <figure
                  v-for="(image, index) in images"
                  :key="image.url + '-' + index"
                  class="group overflow-hidden rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800"
                  :class="images.length === 1 ? 'flex min-h-0 min-w-0 flex-1 flex-col' : ''"
                >
                  <div
                    class="relative flex items-center justify-center overflow-hidden bg-gray-100 dark:bg-dark-950"
                    :class="images.length === 1 ? 'studio-single-media min-h-[520px] flex-1' : 'min-h-[260px]'"
                    :style="images.length > 1 ? { aspectRatio: displayPreviewAspectRatio } : undefined"
                  >
                    <img
                      v-if="!imageLoadErrors[index]"
                      :src="imageSource(image.url, index)"
                      :alt="t('imageStudio.imageAlt', { index: index + 1 })"
                      class="max-h-full max-w-full object-contain"
                      loading="eager"
                      @error="markImageError(index)"
                    />
                    <div v-else class="flex flex-col items-center text-center text-gray-500 dark:text-gray-400">
                      <Icon name="exclamationTriangle" size="lg" class="mb-2" />
                      <span class="text-sm">{{ t('imageStudio.imageLoadFailed') }}</span>
                      <button type="button" class="mt-3 inline-flex items-center gap-1 rounded-md border border-gray-200 px-3 py-1.5 text-xs font-medium hover:bg-white dark:border-dark-600 dark:hover:bg-dark-800" @click="reloadImage(index)">
                        <Icon name="refresh" size="xs" />
                        {{ t('imageStudio.reload') }}
                      </button>
                    </div>
                    <div v-if="!imageLoadErrors[index]" class="absolute right-3 top-3 flex gap-2 opacity-100 transition-opacity sm:opacity-0 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100">
                      <button type="button" class="flex h-9 w-9 items-center justify-center rounded-md bg-black/65 text-white hover:bg-black/80" :title="t('imageStudio.preview')" @click="openPreview(index)">
                        <Icon name="eye" size="sm" />
                      </button>
                      <button type="button" class="flex h-9 w-9 items-center justify-center rounded-md bg-black/65 text-white hover:bg-black/80" :title="t('imageStudio.download')" @click="downloadImage(image.url, index)">
                        <Icon name="download" size="sm" />
                      </button>
                    </div>
                  </div>
                  <figcaption class="flex min-h-11 items-center justify-between gap-3 px-3 py-2">
                    <span class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('imageStudio.resultNumber', { index: index + 1 }) }}</span>
                    <span class="text-xs text-gray-400">{{ displayResolution }} · {{ displayRatio }}</span>
                  </figcaption>
                </figure>
              </div>
            </div>
          </section>
        </main>

        <aside class="studio-history xl:col-span-2 2xl:col-span-1">
          <div class="mb-2 flex min-h-8 items-center justify-between gap-2">
            <div class="flex items-center gap-2">
              <h2 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('imageStudio.history') }}</h2>
              <span class="text-xs tabular-nums text-gray-400">{{ history.length }}</span>
            </div>
            <button
              type="button"
              class="flex h-8 w-8 items-center justify-center rounded-md text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-30 dark:hover:bg-dark-800 dark:hover:text-gray-200"
              :disabled="history.length === 0 || isWorking"
              :title="t('imageStudio.clearHistory')"
              :aria-label="t('imageStudio.clearHistory')"
              @click="clearHistory"
            >
              <Icon name="trash" size="sm" />
            </button>
          </div>

          <section class="studio-history-panel overflow-hidden rounded-lg border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-900">
            <div v-if="history.length === 0" class="flex min-h-40 flex-col items-center justify-center px-4 text-center text-gray-400">
              <Icon name="clock" size="lg" class="mb-2" />
              <p class="text-xs font-medium">{{ t('imageStudio.historyEmpty') }}</p>
              <p class="mt-1 text-[11px]">{{ t('imageStudio.historyHint') }}</p>
            </div>

            <div v-else class="studio-history-list divide-y divide-gray-100 overflow-y-auto dark:divide-dark-700">
              <button
                v-for="item in history"
                :key="item.id"
                type="button"
                class="flex w-full items-center gap-2.5 border-l-2 px-3 py-3 text-left transition-colors"
                :class="activeHistoryId === item.id ? 'border-l-primary-500 bg-primary-50/60 dark:bg-primary-950/20' : 'border-l-transparent hover:bg-gray-50 dark:hover:bg-dark-800'"
                :disabled="isWorking"
                :title="item.prompt"
                @click="selectHistoryItem(item)"
              >
                <div class="flex h-12 w-12 shrink-0 items-center justify-center overflow-hidden rounded-md bg-gray-100 dark:bg-dark-800">
                  <img
                    v-if="historyThumbnail(item)"
                    :src="historyThumbnail(item)"
                    :alt="t('imageStudio.historyThumbnail')"
                    class="h-full w-full object-cover"
                  />
                  <Icon v-else-if="item.status === 'failed'" name="exclamationCircle" size="md" class="text-red-500" />
                  <Icon v-else name="sparkles" size="md" :class="item.status === 'processing' ? 'animate-pulse text-primary-500' : 'text-gray-400'" />
                </div>
                <div class="min-w-0 flex-1">
                  <p class="truncate text-xs font-medium text-gray-800 dark:text-gray-200">{{ item.prompt }}</p>
                  <div class="mt-1 flex items-center justify-between gap-2 text-[11px] text-gray-400">
                    <span>{{ operationLabel(item.operation) }} · {{ item.resolution }} · {{ item.ratio }}</span>
                    <span>{{ formatHistoryTime(item.createdAt) }}</span>
                  </div>
                  <div class="mt-1 flex items-center gap-1.5 text-[11px]" :class="historyStatusClass(item.status)">
                    <span class="h-1.5 w-1.5 rounded-full" :class="historyDotClass(item.status)" />
                    <span>{{ historyStatusLabel(item.status) }}</span>
                  </div>
                </div>
              </button>
            </div>
          </section>
        </aside>
      </div>
    </div>

    <BaseDialog :show="previewIndex !== null" :title="t('imageStudio.previewTitle')" width="full" @close="previewIndex = null">
      <div v-if="previewImage" class="flex max-h-[76vh] min-h-[320px] items-center justify-center overflow-hidden rounded-lg bg-gray-100 p-2 dark:bg-dark-950">
        <img :src="previewImage.url" :alt="t('imageStudio.previewTitle')" class="max-h-[74vh] max-w-full object-contain" />
      </div>
      <template #footer>
        <div class="flex justify-end">
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
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { userGroupsAPI } from '@/api/groups'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { useAppStore } from '@/stores/app'
import type { ApiKey } from '@/types'
import { listEligibleImageKeys } from './access'
import {
  classifyExternalRequestFailure,
  extractTaskImageData,
  generateExternalImageEdit,
  generateExternalImages,
  getImageTask,
  listImageModels,
  normalizeExternalBaseUrl,
  submitImageEditTask,
  submitImageTask,
} from './api'
import {
  buildGenerateImageRequest,
  filterImageModels,
  getAspectRatioOptions,
  getOpenAIImageSize,
  getPreviewAspectRatio,
  getResolutionOptions,
  normalizeStudioSelection,
} from './capabilities'
import { estimateImageCost, formatUSD, getImagePriceTiers } from './pricing'
import type {
  GenerateImageRequest,
  ImageAspectRatio,
  ImageQuality,
  ImageResolutionTier,
  ImageStudioError,
  ImageStudioPlatform,
  ImageTask,
  StoredImageTask,
  StudioOperation,
} from './types'

type ConnectionMode = 'group' | 'external'
type HistoryStatus = 'processing' | 'completed' | 'failed'

interface StudioHistoryItem {
  id: string
  mode: ConnectionMode
  operation: StudioOperation
  prompt: string
  model: string
  resolution: ImageResolutionTier
  ratio: ImageAspectRatio
  quantity: number
  createdAt: number
  status: HistoryStatus
  task?: ImageTask
  thumbnail?: string
  sourceThumbnail?: string
  errorMessage?: string
}

const STORAGE_KEY = 'image_studio_active_task_v1'
const POLL_INTERVAL_MS = 3000
const MAX_HISTORY_ITEMS = 10
const MAX_SOURCE_IMAGE_BYTES = 20 * 1024 * 1024
const SOURCE_IMAGE_TYPES = new Set(['image/png', 'image/jpeg', 'image/webp'])

const { t } = useI18n()
const appStore = useAppStore()

const connectionMode = ref<ConnectionMode>('group')
const operation = ref<StudioOperation>('generate')
const sourceImageFile = ref<File | null>(null)
const sourcePreviewUrl = ref('')
const sourceInput = ref<HTMLInputElement | null>(null)
const sourceDragActive = ref(false)
const apiKeys = ref<ApiKey[]>([])
const userRates = ref<Record<number, number>>({})
const selectedKeyId = ref<number | null>(null)
const models = ref<Array<{ id: string; display_name?: string }>>([])
const internalModel = ref('')
const externalModel = ref('gpt-image-2')
const externalBaseUrl = ref(window.location.origin + '/v1')
const externalApiKey = ref('')
const externalKeyVisible = ref(false)
const prompt = ref('')
const quality = ref<ImageQuality>('auto')
const resolution = ref<ImageResolutionTier>('1K')
const ratio = ref<ImageAspectRatio>('1:1')
const quantity = ref(1)
const loadingKeys = ref(true)
const loadingModels = ref(false)
const submitting = ref(false)
const task = ref<ImageTask | null>(null)
const viewState = ref<'idle' | 'processing' | 'completed' | 'failed'>('idle')
const errorMessage = ref('')
const lastRequest = ref<GenerateImageRequest | null>(null)
const startedAt = ref(0)
const now = ref(Date.now())
const previewIndex = ref<number | null>(null)
const imageLoadErrors = ref<Record<number, boolean>>({})
const imageReloadTokens = ref<Record<number, number>>({})
const history = ref<StudioHistoryItem[]>([])
const activeHistoryId = ref('')
const successfulExternalBaseUrls = new Set<string>()

let historySequence = 0
let modelController: AbortController | null = null
let taskController: AbortController | null = null
let pollTimer: ReturnType<typeof setTimeout> | null = null
let elapsedTimer: ReturnType<typeof setInterval> | null = null

const selectedKey = computed(() => apiKeys.value.find((key) => key.id === selectedKeyId.value) || null)
const selectedGroup = computed(() => connectionMode.value === 'group' ? selectedKey.value?.group || null : null)
const platform = computed<ImageStudioPlatform>(() => {
  if (connectionMode.value === 'external') return 'openai'
  return selectedGroup.value?.platform === 'grok' ? 'grok' : 'openai'
})
const groupBaseUrl = computed(() => window.location.origin + '/v1')
const storageStatusLabel = computed(() => t(
  connectionMode.value === 'external'
    ? 'imageStudio.externalStorageStatus'
    : 'imageStudio.storageStatus',
))
const model = computed({
  get: () => connectionMode.value === 'external' ? externalModel.value : internalModel.value,
  set: (value: string) => {
    if (connectionMode.value === 'external') externalModel.value = value
    else internalModel.value = value
  },
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
  label: item.display_name && item.display_name !== item.id
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
const resolutionOptions = computed(() => getResolutionOptions(platform.value))
const ratioOptions = computed(() => getAspectRatioOptions(platform.value))
const outputSizeLabel = computed(() => platform.value === 'openai'
  ? getOpenAIImageSize(ratio.value, resolution.value).replace('x', ' x ')
  : resolution.value + ' · ' + ratio.value)
const previewAspectRatio = computed(() => getPreviewAspectRatio(ratio.value))
const priceTiers = computed(() => {
  if (!selectedGroup.value) return []
  return getImagePriceTiers(selectedGroup.value, model.value, userRates.value[selectedGroup.value.id])
})
const estimatedCost = computed(() => {
  if (!selectedGroup.value) return 0
  return estimateImageCost(selectedGroup.value, model.value, resolution.value, quantity.value, userRates.value[selectedGroup.value.id])
})
const images = computed(() => task.value ? extractTaskImageData(task.value) : [])
const taskId = computed(() => task.value?.task_id || task.value?.id || '')
const isWorking = computed(() => submitting.value || viewState.value === 'processing')
const canGenerate = computed(() => {
  if (isWorking.value || !model.value.trim() || !prompt.value.trim()) return false
  if (operation.value === 'edit' && (platform.value !== 'openai' || !sourceImageFile.value)) return false
  if (connectionMode.value === 'external') {
    return !!externalBaseUrl.value.trim() && !!externalApiKey.value.trim()
  }
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
const elapsedSeconds = computed(() => startedAt.value > 0 ? Math.max(0, Math.floor((now.value - startedAt.value) / 1000)) : 0)
const statusLabel = computed(() => {
  const editing = statusOperation.value === 'edit'
  if (submitting.value) return t(editing ? 'imageStudio.editSubmitting' : 'imageStudio.submitting')
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
const activeHistoryItem = computed(() => history.value.find((item) => item.id === activeHistoryId.value) || null)
const displayResolution = computed(() => activeHistoryItem.value?.resolution || resolution.value)
const displayRatio = computed(() => activeHistoryItem.value?.ratio || ratio.value)
const displayPreviewAspectRatio = computed(() => getPreviewAspectRatio(displayRatio.value))

function switchOperation(nextOperation: StudioOperation) {
  if (isWorking.value || operation.value === nextOperation) return
  if (nextOperation === 'edit' && platform.value !== 'openai') return
  operation.value = nextOperation
  task.value = null
  activeHistoryId.value = ''
  viewState.value = 'idle'
  errorMessage.value = ''
  previewIndex.value = null
  imageLoadErrors.value = {}
  imageReloadTokens.value = {}
}

function switchConnectionMode(nextMode: ConnectionMode) {
  if (isWorking.value || connectionMode.value === nextMode) return
  modelController?.abort()
  connectionMode.value = nextMode
  if (operation.value === 'edit' && platform.value !== 'openai') operation.value = 'generate'
  task.value = null
  activeHistoryId.value = ''
  viewState.value = 'idle'
  errorMessage.value = ''
  previewIndex.value = null
  imageLoadErrors.value = {}
  imageReloadTokens.value = {}
  quality.value = 'auto'
  const normalized = normalizeStudioSelection(platform.value, ratio.value, resolution.value)
  ratio.value = normalized.ratio
  resolution.value = normalized.resolution
  if (nextMode === 'group' && selectedKey.value && models.value.length === 0) {
    void loadModels()
  }
}

function selectResolution(tier: ImageResolutionTier) {
  if (!resolutionOptions.value.includes(tier)) return
  resolution.value = tier
  if (platform.value === 'openai') {
    ratio.value = tier === '1K' ? '1:1' : (ratio.value === '1:1' ? '3:2' : ratio.value)
  }
}

function selectRatio(nextRatio: ImageAspectRatio) {
  ratio.value = nextRatio
  if (platform.value === 'openai') resolution.value = nextRatio === '1:1' ? '1K' : '2K'
}

async function handleKeySelection() {
  if (connectionMode.value !== 'group' || !selectedKey.value || isWorking.value) return
  if (operation.value === 'edit' && platform.value !== 'openai') operation.value = 'generate'
  const normalized = normalizeStudioSelection(platform.value, ratio.value, resolution.value)
  ratio.value = normalized.ratio
  resolution.value = normalized.resolution
  quality.value = 'auto'
  await loadModels()
}

function openSourcePicker() {
  if (!isWorking.value) sourceInput.value?.click()
}

function isSupportedSourceImage(file: File): boolean {
  if (SOURCE_IMAGE_TYPES.has(file.type.toLowerCase())) return true
  return /[.](png|jpe?g|webp)$/i.test(file.name)
}

function setSourceImage(file: File) {
  sourceDragActive.value = false
  if (!isSupportedSourceImage(file)) {
    appStore.showError(t('imageStudio.sourceImageTypeError'))
    return
  }
  if (file.size > MAX_SOURCE_IMAGE_BYTES) {
    appStore.showError(t('imageStudio.sourceImageSizeError'))
    return
  }
  if (sourcePreviewUrl.value) URL.revokeObjectURL(sourcePreviewUrl.value)
  sourceImageFile.value = file
  sourcePreviewUrl.value = URL.createObjectURL(file)
}

function handleSourceInput(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (file) setSourceImage(file)
  input.value = ''
}

function handleSourceDrop(event: DragEvent) {
  sourceDragActive.value = false
  if (isWorking.value) return
  const file = event.dataTransfer?.files?.[0]
  if (file) setSourceImage(file)
}

function clearSourceImage() {
  if (sourcePreviewUrl.value) URL.revokeObjectURL(sourcePreviewUrl.value)
  sourceImageFile.value = null
  sourcePreviewUrl.value = ''
  if (sourceInput.value) sourceInput.value.value = ''
}

function formatFileSize(bytes: number): string {
  return (bytes / 1024 / 1024).toFixed(bytes >= 1024 * 1024 ? 1 : 2) + ' MB'
}

function processingItemLabel(index: number): string {
  const key = operation.value === 'edit' ? 'imageStudio.editRendering' : 'imageStudio.rendering'
  return t(key, { current: index, total: quantity.value })
}

function operationLabel(value: StudioOperation): string {
  return t(value === 'edit' ? 'imageStudio.editShort' : 'imageStudio.generateShort')
}

async function loadModels(preferredModel = '') {
  modelController?.abort()
  models.value = []
  internalModel.value = preferredModel
  const key = selectedKey.value
  if (!key || connectionMode.value !== 'group') return
  const controller = new AbortController()
  modelController = controller
  loadingModels.value = true
  try {
    const response = await listImageModels(key.key, controller.signal)
    const filtered = filterImageModels(platform.value, response.data || [])
    models.value = filtered
    if (!filtered.some((item) => item.id === preferredModel)) {
      internalModel.value = filtered[0]?.id || ''
    }
  } catch (error) {
    if ((error as Error).name !== 'AbortError') {
      internalModel.value = ''
      appStore.showError(errorText(error, t('imageStudio.modelsFailed')))
    }
  } finally {
    if (modelController === controller) {
      loadingModels.value = false
      modelController = null
    }
  }
}

function stopPolling() {
  taskController?.abort()
  taskController = null
  if (pollTimer) clearTimeout(pollTimer)
  pollTimer = null
}

function persistTask(stored: StoredImageTask) {
  sessionStorage.setItem(STORAGE_KEY, JSON.stringify(stored))
}

function clearPersistedTask() {
  sessionStorage.removeItem(STORAGE_KEY)
}

function readPersistedTask(): StoredImageTask | null {
  try {
    const raw = sessionStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    const value = JSON.parse(raw) as StoredImageTask
    if (!value.taskId || !Number.isFinite(value.apiKeyId) || !value.request?.model) return null
    return value
  } catch {
    clearPersistedTask()
    return null
  }
}

function beginHistoryItem(request: GenerateImageRequest): StudioHistoryItem {
  const item: StudioHistoryItem = {
    id: String(Date.now()) + '-' + String(++historySequence),
    mode: connectionMode.value,
    operation: operation.value,
    prompt: request.prompt,
    model: request.model,
    resolution: resolution.value,
    ratio: ratio.value,
    quantity: request.n,
    createdAt: Date.now(),
    status: 'processing',
    sourceThumbnail: operation.value === 'edit' && sourceImageFile.value
      ? URL.createObjectURL(sourceImageFile.value)
      : undefined,
  }
  const nextHistory = [item, ...history.value]
  nextHistory.slice(MAX_HISTORY_ITEMS).forEach((entry) => {
    if (entry.sourceThumbnail) URL.revokeObjectURL(entry.sourceThumbnail)
  })
  history.value = nextHistory.slice(0, MAX_HISTORY_ITEMS)
  activeHistoryId.value = item.id
  return item
}

function updateHistoryItem(id: string, patch: Partial<StudioHistoryItem>) {
  if (!id) return
  history.value = history.value.map((item) => item.id === id ? { ...item, ...patch } : item)
}

function completeHistoryItem(id: string, completedTask: ImageTask) {
  const firstImage = extractTaskImageData(completedTask)[0]
  const item = history.value.find((entry) => entry.id === id)
  if (item?.sourceThumbnail) URL.revokeObjectURL(item.sourceThumbnail)
  updateHistoryItem(id, {
    status: 'completed',
    task: completedTask,
    thumbnail: firstImage?.url,
    sourceThumbnail: undefined,
    errorMessage: '',
  })
}

function selectHistoryItem(item: StudioHistoryItem) {
  if (isWorking.value || item.status === 'processing') return
  activeHistoryId.value = item.id
  task.value = item.task || null
  viewState.value = item.status
  errorMessage.value = item.errorMessage || ''
  startedAt.value = item.createdAt
  now.value = Date.now()
  previewIndex.value = null
  imageLoadErrors.value = {}
  imageReloadTokens.value = {}
}

function clearHistory() {
  if (isWorking.value) return
  history.value.forEach((item) => {
    if (item.sourceThumbnail) URL.revokeObjectURL(item.sourceThumbnail)
  })
  history.value = []
  activeHistoryId.value = ''
}

function historyThumbnail(item: StudioHistoryItem): string {
  return item.thumbnail || item.sourceThumbnail || ''
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

function formatHistoryTime(timestamp: number): string {
  return new Date(timestamp).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  })
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
  })
}

async function generateImages() {
  if (!canGenerate.value) return
  const external = connectionMode.value === 'external'
  const editing = operation.value === 'edit'
  const sourceFile = sourceImageFile.value
  const key = selectedKey.value
  if (!external && !key) return
  if (editing && !sourceFile) return

  stopPolling()
  task.value = null
  imageLoadErrors.value = {}
  imageReloadTokens.value = {}
  errorMessage.value = ''
  previewIndex.value = null
  const request = currentRequest()
  const historyItem = beginHistoryItem(request)
  lastRequest.value = request
  startedAt.value = historyItem.createdAt
  now.value = startedAt.value
  viewState.value = 'processing'
  submitting.value = true
  const controller = new AbortController()
  taskController = controller

  try {
    if (external) {
      const result = editing
        ? await generateExternalImageEdit(
            externalBaseUrl.value,
            externalApiKey.value,
            sourceFile as File,
            request,
            controller.signal,
          )
        : await generateExternalImages(
            externalBaseUrl.value,
            externalApiKey.value,
            request,
            controller.signal,
          )
      const completedAt = Math.floor(Date.now() / 1000)
      const completedTask: ImageTask = {
        id: '',
        task_id: '',
        object: editing ? 'image.edit' : 'image.generation',
        status: 'completed',
        result,
        created_at: Math.floor(startedAt.value / 1000),
        completed_at: completedAt,
        expires_at: 0,
      }
      successfulExternalBaseUrls.add(normalizedExternalBaseUrl())
      if (extractTaskImageData(completedTask).length === 0) {
        failTask(t('imageStudio.emptyResult'))
        return
      }
      task.value = completedTask
      viewState.value = 'completed'
      completeHistoryItem(historyItem.id, completedTask)
      taskController = null
      appStore.showSuccess(t(editing ? 'imageStudio.editSuccess' : 'imageStudio.generateSuccess'))
      return
    }

    if (!key) return
    const submitted = editing
      ? await submitImageEditTask(key.key, sourceFile as File, request, controller.signal)
      : await submitImageTask(key.key, request, controller.signal)
    task.value = submitted
    updateHistoryItem(historyItem.id, { task: submitted })
    persistTask({
      taskId: submitted.task_id || submitted.id,
      apiKeyId: key.id,
      operation: operation.value,
      request,
      platform: platform.value,
      ratio: ratio.value,
      resolution: resolution.value,
      startedAt: startedAt.value,
    })
    submitting.value = false
    await pollTask(submitted.task_id || submitted.id, key)
  } catch (error) {
    if ((error as Error).name === 'AbortError') return
    failTask(external
      ? externalErrorText(error)
      : errorText(error, t(editing ? 'imageStudio.editSubmitFailed' : 'imageStudio.submitFailed')))
  } finally {
    submitting.value = false
  }
}
async function pollTask(id: string, key: ApiKey) {
  const controller = new AbortController()
  taskController = controller
  try {
    const current = await getImageTask(key.key, id, controller.signal)
    task.value = current
    updateHistoryItem(activeHistoryId.value, { task: current })
    if (current.status === 'completed') {
      if (extractTaskImageData(current).length === 0) {
        failTask(t('imageStudio.emptyResult'))
        return
      }
      viewState.value = 'completed'
      completeHistoryItem(activeHistoryId.value, current)
      clearPersistedTask()
      taskController = null
      const completedOperation = history.value.find((item) => item.id === activeHistoryId.value)?.operation || operation.value
      appStore.showSuccess(t(completedOperation === 'edit' ? 'imageStudio.editSuccess' : 'imageStudio.generateSuccess'))
      return
    }
    if (current.status === 'failed') {
      const failedOperation = history.value.find((item) => item.id === activeHistoryId.value)?.operation || operation.value
      failTask(taskErrorText(current.error) || t(failedOperation === 'edit' ? 'imageStudio.editGenerateFailed' : 'imageStudio.generateFailed'))
      return
    }
    viewState.value = 'processing'
    pollTimer = setTimeout(() => pollTask(id, key), POLL_INTERVAL_MS)
  } catch (error) {
    if ((error as Error).name === 'AbortError') return
    failTask(errorText(error, t('imageStudio.pollFailed')))
  }
}

function failTask(message: string) {
  viewState.value = 'failed'
  errorMessage.value = message
  updateHistoryItem(activeHistoryId.value, {
    status: 'failed',
    task: task.value || undefined,
    errorMessage: message,
  })
  clearPersistedTask()
  stopPolling()
}

function taskErrorText(error: unknown): string {
  if (!error) return ''
  if (typeof error === 'string') return error
  if (typeof error === 'object') {
    const value = error as { message?: string; error?: { message?: string } }
    return value.message || value.error?.message || ''
  }
  return ''
}

function normalizedExternalBaseUrl(): string {
  try {
    return normalizeExternalBaseUrl(externalBaseUrl.value)
  } catch {
    return externalBaseUrl.value.trim()
  }
}

function externalErrorText(error: unknown): string {
  const failureKind = classifyExternalRequestFailure(
    error,
    Date.now() - startedAt.value,
    successfulExternalBaseUrls.has(normalizedExternalBaseUrl()),
  )
  if (failureKind === 'invalid-url') return t('imageStudio.externalInvalidUrl')
  if (failureKind === 'connection-interrupted') return t('imageStudio.externalConnectionInterrupted')
  if (failureKind === 'network') return t('imageStudio.externalCorsError')
  return errorText(error, t('imageStudio.externalRequestFailed'))
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

async function restoreTask(stored: StoredImageTask) {
  const key = apiKeys.value.find((item) => item.id === stored.apiKeyId)
  if (!key) {
    clearPersistedTask()
    return false
  }
  connectionMode.value = 'group'
  operation.value = stored.operation || 'generate'
  selectedKeyId.value = key.id
  prompt.value = stored.request.prompt || ''
  internalModel.value = stored.request.model || ''
  quantity.value = Math.min(4, Math.max(1, stored.request.n || 1))
  quality.value = stored.request.quality || 'auto'
  ratio.value = stored.ratio || '1:1'
  resolution.value = stored.resolution || '1K'
  lastRequest.value = stored.request
  startedAt.value = stored.startedAt || Date.now()
  viewState.value = 'processing'
  task.value = {
    id: stored.taskId,
    task_id: stored.taskId,
    object: operation.value === 'edit' ? 'image.edit.task' : 'image.generation.task',
    status: 'processing',
    created_at: Math.floor(startedAt.value / 1000),
    expires_at: 0,
  }
  const restoredHistoryItem = beginHistoryItem(stored.request)
  updateHistoryItem(restoredHistoryItem.id, {
    createdAt: startedAt.value,
    task: task.value,
  })
  await loadModels(stored.request.model)
  await pollTask(stored.taskId, key)
  return true
}

onMounted(async () => {
  elapsedTimer = setInterval(() => { now.value = Date.now() }, 1000)
  try {
    const [keys, rates] = await Promise.all([
      listEligibleImageKeys(),
      userGroupsAPI.getUserGroupRates().catch(() => ({})),
    ])
    apiKeys.value = keys
    userRates.value = rates
    const stored = readPersistedTask()
    if (stored && await restoreTask(stored)) return
    selectedKeyId.value = keys[0]?.id || null
    await loadModels()
  } catch (error) {
    appStore.showError(errorText(error, t('imageStudio.keysFailed')))
  } finally {
    loadingKeys.value = false
  }
})

onBeforeUnmount(() => {
  modelController?.abort()
  stopPolling()
  if (elapsedTimer) clearInterval(elapsedTimer)
  if (sourcePreviewUrl.value) URL.revokeObjectURL(sourcePreviewUrl.value)
  history.value.forEach((item) => {
    if (item.sourceThumbnail) URL.revokeObjectURL(item.sourceThumbnail)
  })
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

.studio-single-media {
  height: clamp(520px, calc(100vh - 17rem), 900px);
  flex: none;
}

@media (min-width: 1280px) {
  .studio-canvas,
  .studio-canvas > div {
    min-height: max(600px, calc(100vh - 13.5rem));
  }
}

@media (min-width: 1536px) {
  .studio-history-panel {
    height: max(600px, calc(100vh - 13.5rem));
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
