<template>
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
    <div v-else v-html="homeContent"></div>
  </div>

  <div
    v-else
    class="landing-shell relative min-h-screen overflow-hidden text-slate-950 dark:text-slate-100"
    :class="{ 'is-dark': isDark }"
    :style="landingShellStyle"
  >
    <div class="landing-noise pointer-events-none absolute inset-0"></div>
    <div class="landing-grid pointer-events-none absolute inset-0"></div>
    <div class="landing-spot landing-spot-a pointer-events-none absolute"></div>
    <div class="landing-spot landing-spot-b pointer-events-none absolute"></div>
    <div class="landing-spot landing-spot-c pointer-events-none absolute"></div>

    <header class="relative z-20 px-4 pt-4 sm:px-6 lg:px-10">
      <nav
        class="mx-auto flex max-w-[1280px] items-center justify-between rounded-full border border-white/65 bg-white/78 px-3 py-3 shadow-[0_20px_70px_rgba(15,23,42,0.08)] backdrop-blur-2xl dark:border-white/10 dark:bg-slate-950/72 dark:shadow-[0_24px_90px_rgba(0,0,0,0.34)]"
      >
        <router-link to="/home" class="flex min-w-0 items-center gap-3">
          <div
            class="flex h-11 w-11 items-center justify-center overflow-hidden rounded-2xl border border-black/5 shadow-[0_14px_40px_rgba(15,23,42,0.12)] dark:border-white/10 dark:bg-slate-900"
          >
            <img
              v-if="siteLogo"
              :src="siteLogo"
              :alt="siteName"
              :class="['h-full w-full object-contain p-1.5', isDark ? 'site-logo-dark' : 'site-logo-light']"
            />

          </div>

          <div class="min-w-0">
            <p
              class="brand-wordmark truncate text-sm font-semibold tracking-[-0.04em] text-slate-950 dark:text-white sm:text-base"
            >
              {{ siteName }}
            </p>
            <p class="truncate text-[11px] uppercase tracking-[0.24em] text-slate-500 dark:text-slate-400">
              AI Gateway
            </p>
          </div>
        </router-link>

        <div class="flex items-center gap-2 sm:gap-3">
          <LocaleSwitcher />

          <button
            @click="openAnnouncementModal"
            class="relative inline-flex h-10 items-center justify-center gap-2 rounded-full border border-black/5 bg-black/[0.03] px-3 text-sm font-medium text-slate-600 transition duration-300 hover:-translate-y-0.5 hover:bg-black/[0.06] hover:text-slate-950 dark:border-white/10 dark:bg-slate-900/70 dark:text-slate-300 dark:hover:bg-slate-800/80 dark:hover:text-white"
            :title="copy.announcementLabel"
          >
            <span
              v-if="showAnnouncementDot"
              class="absolute right-2 top-2 flex h-2.5 w-2.5"
            >
              <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-500 opacity-75"></span>
              <span class="relative inline-flex h-2.5 w-2.5 rounded-full bg-red-500"></span>
            </span>
            <Icon name="bell" size="sm" />
            <span class="hidden md:inline">{{ copy.announcementLabel }}</span>
          </button>

          <button
            @click="serviceModalOpen = true"
            class="inline-flex h-10 items-center justify-center gap-2 rounded-full border border-black/5 bg-black/[0.03] px-3 text-sm font-medium text-slate-600 transition duration-300 hover:-translate-y-0.5 hover:bg-black/[0.06] hover:text-slate-950 dark:border-white/10 dark:bg-slate-900/70 dark:text-slate-300 dark:hover:bg-slate-800/80 dark:hover:text-white"
            :title="copy.serviceLabel"
          >
            <Icon name="chatBubble" size="sm" />
            <span class="hidden md:inline">{{ copy.serviceLabel }}</span>
          </button>

          <router-link
            to="/tutorial"
            class="hidden h-10 w-10 items-center justify-center rounded-full border border-black/5 bg-black/[0.03] text-slate-600 transition duration-300 hover:-translate-y-0.5 hover:bg-black/[0.06] hover:text-slate-950 dark:border-white/10 dark:bg-slate-900/70 dark:text-slate-300 dark:hover:bg-slate-800/80 dark:hover:text-white md:inline-flex"
            :title="copy.docsLabel"
          >
            <Icon name="book" size="md" />
          </router-link>

          <button
            @click="toggleTheme"
            class="inline-flex h-10 w-10 items-center justify-center rounded-full border border-black/5 bg-black/[0.03] text-slate-600 transition duration-300 hover:-translate-y-0.5 hover:bg-black/[0.06] hover:text-slate-950 dark:border-white/10 dark:bg-slate-900/70 dark:text-slate-300 dark:hover:bg-slate-800/80 dark:hover:text-white"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="md" />
            <Icon v-else name="moon" size="md" />
          </button>

          <router-link
            :to="primaryActionPath"
            class="hidden items-center gap-2 rounded-full bg-slate-950 px-4 py-2.5 text-sm font-medium text-white shadow-[0_18px_40px_rgba(2,6,23,0.16)] transition duration-300 hover:-translate-y-0.5 hover:bg-black dark:bg-white/80 dark:text-slate-950 dark:hover:bg-white/90 md:inline-flex"
          >
            <span
              v-if="isAuthenticated"
              class="flex h-6 w-6 items-center justify-center rounded-full bg-gradient-to-br from-cyan-300 to-teal-500 text-[11px] font-semibold text-slate-950"
            >
              {{ userInitial }}
            </span>
            <span>{{ primaryActionLabel }}</span>
            <Icon name="arrowRight" size="sm" class="hidden sm:block" />
          </router-link>
        </div>
      </nav>
    </header>

    <main class="relative z-10 px-4 pb-14 pt-8 sm:px-6 lg:px-10 lg:pb-20">
      <div class="mx-auto flex max-w-[1280px] flex-col gap-6 lg:gap-8">
        <section class="grid items-center gap-10 lg:grid-cols-[minmax(0,0.96fr)_minmax(0,1.04fr)] lg:gap-14">
          <div class="max-w-2xl">
            <div
              class="inline-flex items-center gap-2 rounded-full border border-black/5 bg-white/72 px-3 py-1.5 text-[11px] font-medium uppercase tracking-[0.22em] text-slate-500 shadow-[0_12px_32px_rgba(15,23,42,0.06)] backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/60 dark:text-slate-300 dark:shadow-[0_12px_40px_rgba(0,0,0,0.25)]"
            >
              <span class="inline-flex h-2 w-2 rounded-full bg-teal-400"></span>
              {{ copy.eyebrow }}
            </div>

            <p
              class="brand-wordmark mt-6 text-sm font-semibold uppercase tracking-[0.24em] text-slate-500 dark:text-slate-400"
            >
              {{ siteName }}
            </p>

            <h1
              class="landing-display mt-4 whitespace-nowrap text-[clamp(2.35rem,4vw,4.2rem)] font-semibold leading-[1.02] tracking-[-0.065em] text-slate-950 dark:text-white"
            >
              <span
                class="inline-block bg-gradient-to-r from-slate-950 via-teal-600 to-sky-500 bg-clip-text text-transparent dark:from-white dark:via-sky-300 dark:to-teal-300"
              >
                {{ copy.heroTitle }}
              </span>
            </h1>

            <p class="mt-6 max-w-xl text-base leading-8 text-slate-600 dark:text-slate-300 sm:text-lg">
              {{ heroLead }}
            </p>

            <div class="mt-8 flex flex-wrap items-center gap-3">
              <router-link
                :to="primaryActionPath"
                class="inline-flex items-center gap-2 rounded-full bg-slate-950 px-6 py-3 text-sm font-medium text-white shadow-[0_20px_48px_rgba(15,23,42,0.16)] transition duration-300 hover:-translate-y-0.5 hover:bg-black dark:bg-white/80 dark:text-slate-950 dark:hover:bg-white/90"
              >
                {{ primaryActionLabel }}
                <Icon name="arrowRight" size="sm" />
              </router-link>

              <router-link
                to="/tutorial"
                class="inline-flex items-center gap-2 rounded-full border border-black/8 bg-white/78 px-6 py-3 text-sm font-medium text-slate-700 shadow-[0_16px_40px_rgba(15,23,42,0.08)] backdrop-blur-xl transition duration-300 hover:-translate-y-0.5 hover:border-black/12 hover:text-slate-950 dark:border-white/10 dark:bg-slate-900/68 dark:text-slate-200 dark:shadow-[0_18px_44px_rgba(0,0,0,0.25)] dark:hover:border-white/20 dark:hover:text-white"
              >
                {{ copy.docsLabel }}
                <Icon name="chevronRight" size="sm" />
              </router-link>
            </div>

            <div class="mt-8 flex flex-wrap gap-3">
              <div
                v-for="signal in heroSignals"
                :key="signal.label"
                class="inline-flex items-center gap-2 rounded-full border border-black/5 bg-white/68 px-4 py-2 text-xs font-medium text-slate-600 shadow-[0_14px_36px_rgba(15,23,42,0.06)] backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/58 dark:text-slate-300 dark:shadow-[0_14px_40px_rgba(0,0,0,0.22)]"
              >
                <span class="inline-flex h-2 w-2 rounded-full" :style="{ backgroundColor: signal.color }"></span>
                <span>{{ signal.label }}</span>
              </div>
            </div>
          </div>

          <div class="relative">
            <div class="studio-shell mx-auto w-full max-w-[620px]">
              <div class="studio-aura absolute inset-x-12 bottom-10 top-10 rounded-full blur-3xl"></div>

              <div class="studio-panel relative overflow-hidden rounded-[34px] p-4 sm:p-5">
                <div
                  class="studio-head flex items-center justify-between rounded-[24px] border border-white/40 bg-white/60 px-4 py-3 backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/62"
                >
                  <div class="flex items-center gap-2">
                    <span class="h-2.5 w-2.5 rounded-full bg-rose-400"></span>
                    <span class="h-2.5 w-2.5 rounded-full bg-amber-300"></span>
                    <span class="h-2.5 w-2.5 rounded-full bg-emerald-400"></span>
                  </div>
                  <p
                    class="text-[11px] font-medium uppercase tracking-[0.26em] text-slate-500 dark:text-slate-400"
                  >
                    Routing Studio
                  </p>
                </div>

                <div class="studio-stage mt-4 rounded-[28px] border border-white/40 bg-slate-950/[0.04] p-5 dark:border-white/10 dark:bg-black/20">
                  <div class="grid gap-5 lg:grid-cols-[0.88fr_0.54fr_0.78fr] lg:items-center">
                    <div class="space-y-3">
                      <div
                        v-for="provider in providers"
                        :key="provider.name"
                        class="route-pill flex items-center justify-between rounded-2xl border border-white/50 bg-white/75 px-4 py-3 text-sm font-medium text-slate-700 shadow-[0_14px_32px_rgba(15,23,42,0.08)] backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/66 dark:text-slate-100 dark:shadow-[0_12px_28px_rgba(0,0,0,0.22)]"
                      >
                        <div class="flex items-center gap-3">
                          <span class="h-2.5 w-2.5 rounded-full" :style="{ backgroundColor: provider.color }"></span>
                          <span>{{ provider.name }}</span>
                        </div>
                        <span class="text-[10px] uppercase tracking-[0.22em] text-slate-400">live</span>
                      </div>
                    </div>

                    <div class="studio-core relative mx-auto flex h-44 w-32 items-center justify-center">
                      <span class="core-beam core-beam-a"></span>
                      <span class="core-beam core-beam-b"></span>
                      <span class="core-beam core-beam-c"></span>
                      <span class="core-ring core-ring-a"></span>
                      <span class="core-ring core-ring-b"></span>
                      <div
                        class="core-node relative z-10 flex h-24 w-24 flex-col items-center justify-center rounded-full border border-white/55 bg-white/80 text-center shadow-[0_30px_70px_rgba(20,184,166,0.24)] backdrop-blur-2xl dark:border-white/12 dark:bg-slate-900/72 dark:shadow-[0_24px_70px_rgba(56,189,248,0.18)]"
                      >
                        <span class="text-[10px] uppercase tracking-[0.28em] text-slate-400">API</span>
                        <span class="brand-wordmark mt-1 text-sm font-semibold tracking-[-0.04em] text-slate-950 dark:text-white">
                          CORE
                        </span>
                      </div>
                    </div>

                    <div class="space-y-4">
                      <div
                        class="studio-compat rounded-[26px] border border-white/50 bg-white/75 p-4 shadow-[0_18px_40px_rgba(15,23,42,0.08)] backdrop-blur-2xl dark:border-white/10 dark:bg-slate-900/68 dark:shadow-[0_18px_40px_rgba(0,0,0,0.24)]"
                      >
                        <p class="text-[11px] uppercase tracking-[0.24em] text-slate-400">Compatible</p>
                        <p class="mt-3 text-2xl font-semibold tracking-[-0.05em] text-slate-950 dark:text-white">
                          /v1/chat
                        </p>
                        <div class="mt-4 h-px bg-gradient-to-r from-transparent via-slate-300 to-transparent dark:via-slate-700"></div>
                        <div class="mt-4 flex items-center gap-2 text-xs text-slate-500 dark:text-slate-300">
                          <span class="rounded-full bg-slate-950 px-2 py-1 text-[10px] uppercase tracking-[0.18em] text-white dark:bg-white dark:text-slate-950">
                            json
                          </span>
                          <span class="rounded-full border border-black/8 px-2 py-1 dark:border-white/10">
                            stream
                          </span>
                        </div>
                      </div>

                      <div class="grid grid-cols-2 gap-3">
                        <div
                          class="studio-client-card rounded-2xl border border-white/45 bg-white/70 px-4 py-3 text-xs text-slate-500 shadow-[0_14px_30px_rgba(15,23,42,0.06)] backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/60 dark:text-slate-300"
                        >
                          <p class="uppercase tracking-[0.22em] text-slate-400">client</p>
                          <p class="mt-2 text-sm font-semibold text-slate-900 dark:text-white">Codex</p>
                        </div>
                        <div
                          class="studio-client-card rounded-2xl border border-white/45 bg-white/70 px-4 py-3 text-xs text-slate-500 shadow-[0_14px_30px_rgba(15,23,42,0.06)] backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/60 dark:text-slate-300"
                        >
                          <p class="uppercase tracking-[0.22em] text-slate-400">client</p>
                          <p class="mt-2 text-sm font-semibold text-slate-900 dark:text-white">Claude Code</p>
                        </div>
                      </div>
                    </div>
                  </div>

                  <div class="mt-5 grid gap-3 sm:grid-cols-3">
                    <div
                      v-for="(signal, index) in heroSignals"
                      :key="`${signal.label}-${index}`"
                      class="studio-signal-card rounded-2xl border border-white/40 bg-white/60 px-4 py-3 shadow-[0_14px_30px_rgba(15,23,42,0.05)] backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/58 dark:shadow-[0_12px_26px_rgba(0,0,0,0.22)]"
                    >
                      <p class="text-[11px] uppercase tracking-[0.24em] text-slate-400">
                        0{{ index + 1 }}
                      </p>
                      <p class="mt-2 text-sm font-medium text-slate-700 dark:text-slate-100">
                        {{ signal.label }}
                      </p>
                    </div>
                  </div>
                </div>
              </div>

              <div
                class="studio-float studio-float-top rounded-2xl border border-white/50 bg-white/75 px-4 py-3 text-xs font-medium text-slate-700 shadow-[0_18px_38px_rgba(15,23,42,0.08)] backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/66 dark:text-slate-100 dark:shadow-[0_18px_36px_rgba(0,0,0,0.24)]"
              >
                {{ copy.floatOne }}
              </div>

              <div
                class="studio-float studio-float-bottom rounded-2xl border border-white/50 bg-white/75 px-4 py-3 text-xs font-medium text-slate-700 shadow-[0_18px_38px_rgba(15,23,42,0.08)] backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/66 dark:text-slate-100 dark:shadow-[0_18px_36px_rgba(0,0,0,0.24)]"
              >
                {{ copy.floatTwo }}
              </div>
            </div>
          </div>
        </section>

        <section class="grid gap-4 md:grid-cols-3">
          <article
            v-for="feature in featureCards"
            :key="feature.title"
            class="group relative overflow-hidden rounded-[30px] border border-white/60 bg-white/74 p-6 shadow-[0_20px_60px_rgba(15,23,42,0.07)] backdrop-blur-2xl transition duration-300 hover:-translate-y-1 hover:shadow-[0_28px_70px_rgba(15,23,42,0.12)] dark:border-white/10 dark:bg-slate-900/60 dark:shadow-[0_20px_54px_rgba(0,0,0,0.26)] dark:hover:shadow-[0_30px_64px_rgba(0,0,0,0.34)]"
          >
            <div
              class="pointer-events-none absolute inset-x-6 top-0 h-24 rounded-b-full opacity-60 blur-2xl"
              :style="{ background: `radial-gradient(circle, ${feature.color} 0%, transparent 72%)` }"
            ></div>

            <div
              class="relative flex h-14 w-14 items-center justify-center rounded-2xl border border-white/50 bg-white/82 shadow-[0_16px_36px_rgba(15,23,42,0.08)] dark:border-white/10 dark:bg-slate-800/72"
            >
              <Icon :name="feature.icon" size="lg" :style="{ color: feature.color }" />
            </div>

            <p class="relative mt-5 text-[11px] uppercase tracking-[0.24em] text-slate-400">
              {{ feature.kicker }}
            </p>
            <h3 class="relative mt-3 text-xl font-semibold tracking-[-0.04em] text-slate-950 dark:text-white">
              {{ feature.title }}
            </h3>
            <p class="relative mt-3 text-sm leading-7 text-slate-600 dark:text-slate-300">
              {{ feature.description }}
            </p>
          </article>
        </section>
      </div>
    </main>

    <footer class="relative z-10 px-4 pb-8 pt-2 sm:px-6 lg:px-10">
      <div
        class="mx-auto flex max-w-[1280px] flex-col items-center justify-between gap-4 border-t border-black/5 pt-6 text-center dark:border-white/10 md:flex-row md:text-left"
      >
        <p class="text-sm text-slate-500 dark:text-slate-400">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>

        <div class="flex items-center gap-4">
          <button
            @click="serviceModalOpen = true"
            class="text-sm text-slate-500 transition-colors hover:text-slate-950 dark:text-slate-400 dark:hover:text-white"
          >
            {{ copy.serviceLabel }}
          </button>
          <router-link
            to="/tutorial"
            class="text-sm text-slate-500 transition-colors hover:text-slate-950 dark:text-slate-400 dark:hover:text-white"
          >
            {{ copy.docsLabel }}
          </router-link>
        </div>
      </div>
    </footer>

    <Teleport to="body">
      <Transition name="modal-fade">
        <div
          v-if="serviceModalOpen"
          class="fixed inset-0 z-[110] flex items-center justify-center bg-slate-950/72 p-4 backdrop-blur-md"
          @click="serviceModalOpen = false"
        >
          <div
            class="w-full max-w-[760px] overflow-hidden rounded-[32px] border border-white/55 bg-white/58 shadow-[0_30px_90px_rgba(15,23,42,0.18)] backdrop-blur-[28px] dark:border-white/10 dark:bg-slate-950/62 dark:shadow-[0_30px_100px_rgba(0,0,0,0.42)]"
            @click.stop
          >
            <div class="border-b border-black/5 px-6 py-5 dark:border-white/10">
              <div class="flex items-center justify-between gap-4">
                <div>
                  <p class="text-[11px] uppercase tracking-[0.24em] text-slate-400">{{ copy.serviceLabel }}</p>
                  <h3 class="landing-display mt-2 text-2xl font-semibold tracking-[-0.04em] text-slate-950 dark:text-white">
                    {{ copy.serviceTitle }}
                  </h3>
                </div>
                <button
                  class="inline-flex h-10 w-10 items-center justify-center rounded-full border border-black/5 bg-black/[0.03] text-slate-500 transition hover:text-slate-950 dark:border-white/10 dark:bg-slate-800/70 dark:text-slate-300 dark:hover:text-white"
                  @click="serviceModalOpen = false"
                >
                  <Icon name="x" size="md" />
                </button>
              </div>
            </div>

            <div class="grid gap-5 p-6">
              <article
                v-for="card in serviceCards"
                :key="card.title"
                class="rounded-[28px] border border-white/60 bg-white/54 p-5 shadow-[0_18px_54px_rgba(15,23,42,0.08)] backdrop-blur-[24px] dark:border-white/10 dark:bg-slate-900/58 dark:shadow-[0_18px_44px_rgba(0,0,0,0.28)]"
              >
                <div class="flex items-center gap-3">
                  <div
                    class="flex h-12 w-12 items-center justify-center rounded-2xl border border-white/60 bg-white/70 dark:border-white/10 dark:bg-slate-800/70"
                  >
                    <Icon :name="card.icon" size="lg" :style="{ color: card.color }" />
                  </div>
                  <div>
                    <p class="text-lg font-semibold tracking-[-0.04em] text-slate-950 dark:text-white">
                      {{ card.title }}
                    </p>
                    <p class="text-sm text-slate-500 dark:text-slate-400">{{ card.desc }}</p>
                  </div>
                </div>

                <div class="mt-5 rounded-[24px] border border-dashed border-black/10 bg-white/44 p-4 backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/56">
                  <div class="qr-frame mx-auto flex items-center justify-center">
                    <img
                      v-if="card.qrUrl"
                      :src="card.qrUrl"
                      :alt="card.title"
                      class="h-[180px] w-[180px] rounded-[24px] object-cover"
                    />
                    <div
                      v-else
                      class="flex h-[180px] w-[180px] items-center justify-center rounded-[24px] border border-dashed border-slate-300/80 px-6 text-center text-sm leading-6 text-slate-500 dark:border-slate-700 dark:text-slate-400"
                    >
                      {{ isZh ? '先填入客服 URL 后，这里会自动生成二维码。' : 'Add the support URL first and the QR code will be generated here.' }}
                    </div>
                  </div>
                </div>

              </article>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <Teleport to="body">
      <Transition name="modal-fade">
        <div
          v-if="announcementModalOpen"
          class="fixed inset-0 z-[115] flex items-center justify-center bg-slate-950/72 p-4 backdrop-blur-md"
          @click="announcementModalOpen = false"
        >
          <div
            class="w-full max-w-[760px] overflow-hidden rounded-[32px] border border-white/55 bg-white/58 shadow-[0_30px_90px_rgba(15,23,42,0.18)] backdrop-blur-[28px] dark:border-white/10 dark:bg-slate-950/62 dark:shadow-[0_30px_100px_rgba(0,0,0,0.42)]"
            @click.stop
          >
            <div class="border-b border-black/5 px-6 py-5 dark:border-white/10">
              <div class="flex items-center justify-between gap-4">
                <div>
                  <p class="text-[11px] uppercase tracking-[0.24em] text-slate-400">{{ copy.announcementLabel }}</p>
                  <h3 class="landing-display mt-2 text-2xl font-semibold tracking-[-0.04em] text-slate-950 dark:text-white">
                    {{ copy.announcementTitle }}
                  </h3>
                </div>
                <button
                  class="inline-flex h-10 w-10 items-center justify-center rounded-full border border-black/5 bg-black/[0.03] text-slate-500 transition hover:text-slate-950 dark:border-white/10 dark:bg-slate-800/70 dark:text-slate-300 dark:hover:text-white"
                  @click="announcementModalOpen = false"
                >
                  <Icon name="x" size="md" />
                </button>
              </div>
            </div>

            <div class="max-h-[68vh] overflow-y-auto p-4 md:p-5">
              <div v-if="announcementLoading" class="flex items-center justify-center py-12">
                <div class="h-10 w-10 animate-spin rounded-full border-4 border-slate-200 border-t-teal-500 dark:border-slate-700 dark:border-t-sky-400"></div>
              </div>

              <div
                v-else-if="visibleAnnouncements.length === 0"
                class="rounded-[28px] border border-white/60 bg-white/80 p-6 text-center shadow-[0_18px_54px_rgba(15,23,42,0.08)] backdrop-blur-xl dark:border-white/10 dark:bg-slate-900/60 dark:shadow-[0_18px_44px_rgba(0,0,0,0.28)]"
              >
                <p class="text-lg font-semibold tracking-[-0.04em] text-slate-950 dark:text-white">
                  {{ copy.announcementEmpty }}
                </p>
                <p class="mt-3 text-sm leading-7 text-slate-600 dark:text-slate-300">
                  {{ copy.announcementEmptyDesc }}
                </p>
              </div>

              <div v-else class="space-y-3">
                <article
                  v-for="item in visibleAnnouncements"
                  :key="item.id"
                  class="group rounded-[28px] border border-white/60 bg-white/78 p-5 shadow-[0_18px_54px_rgba(15,23,42,0.08)] backdrop-blur-xl transition duration-300 hover:-translate-y-0.5 dark:border-white/10 dark:bg-slate-900/60 dark:shadow-[0_18px_44px_rgba(0,0,0,0.28)]"
                  :class="{ 'ring-1 ring-red-400/30 dark:ring-red-400/20': !item.read_at }"
                >
                  <div class="flex items-start justify-between gap-4">
                    <div>
                      <div class="flex items-center gap-2">
                        <span
                          v-if="!item.read_at"
                          class="relative flex h-2.5 w-2.5"
                        >
                          <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-red-500 opacity-75"></span>
                          <span class="relative inline-flex h-2.5 w-2.5 rounded-full bg-red-500"></span>
                        </span>
                        <h4 class="text-lg font-semibold tracking-[-0.04em] text-slate-950 dark:text-white">
                          {{ item.title }}
                        </h4>
                      </div>
                      <div class="mt-2 flex items-center gap-3 text-xs text-slate-400 dark:text-slate-500">
                        <span>{{ formatRelativeWithDateTime(item.created_at) }}</span>
                        <span>{{ item.read_at ? copy.announcementRead : copy.announcementUnread }}</span>
                      </div>
                    </div>

                    <button
                      v-if="!item.read_at"
                      class="rounded-full border border-black/8 bg-white/80 px-3 py-1.5 text-xs font-medium text-slate-600 transition hover:text-slate-950 dark:border-white/10 dark:bg-slate-800/70 dark:text-slate-300 dark:hover:text-white"
                      @click="markAnnouncementRead(item.id)"
                    >
                      {{ copy.markReadLabel }}
                    </button>
                  </div>

                  <div
                    class="announcement-body mt-4 prose prose-sm max-w-none text-slate-700 dark:prose-invert dark:text-slate-300"
                    v-html="renderAnnouncement(item.content)"
                  ></div>
                </article>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { storeToRefs } from 'pinia'
import { useAnnouncementStore, useAppStore, useAuthStore } from '@/stores'
import { formatRelativeWithDateTime } from '@/utils/format'
import type { UserAnnouncement } from '@/types'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

const { t, locale } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()
const announcementStore = useAnnouncementStore()
const { announcements, loading: storeAnnouncementLoading } = storeToRefs(announcementStore)

const DEFAULT_SITE_SUBTITLE = 'AI API Gateway Platform'
const FRONTEND_SUPPORT_QR_IMAGE_URL = 'https://www.weidus.com/wp-content/uploads/2026/05/1779695656-QQ20260525-155015.png'

const siteName = computed(() => {
  const configuredName = (appStore.cachedPublicSettings?.site_name || appStore.siteName || '').trim()
  if (!configuredName || configuredName.toLowerCase() === 'sub2api') return 'Z-API'
  return configuredName
})
const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '')
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || DEFAULT_SITE_SUBTITLE)
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')
const registrationEnabled = computed(() => appStore.cachedPublicSettings?.registration_enabled ?? false)
const supportQrImageUrl = computed(() => FRONTEND_SUPPORT_QR_IMAGE_URL.trim())

const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isDark = ref(document.documentElement.classList.contains('dark'))
const serviceModalOpen = ref(false)
const announcementModalOpen = ref(false)
const publicAnnouncements = ref<UserAnnouncement[]>([])
const publicAnnouncementLoading = ref(false)
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => (isAdmin.value ? '/admin/dashboard' : '/dashboard'))
const userInitial = computed(() => {
  const user = authStore.user
  if (!user?.email) return ''
  return user.email.charAt(0).toUpperCase()
})


const currentYear = computed(() => new Date().getFullYear())
const isZh = computed(() => locale.value.startsWith('zh'))
const unreadCount = computed(() => announcementStore.unreadCount)
const showAnnouncementDot = computed(() => isAuthenticated.value && unreadCount.value > 0)
const visibleAnnouncements = computed(() =>
  isAuthenticated.value ? announcements.value : publicAnnouncements.value
)
const announcementLoading = computed(() =>
  isAuthenticated.value ? storeAnnouncementLoading.value : publicAnnouncementLoading.value
)
const landingShellStyle = computed(() => ({
  background: isDark.value
    ? 'radial-gradient(circle at 12% 14%, rgba(20, 184, 166, 0.12), transparent 24%), radial-gradient(circle at 82% 18%, rgba(56, 189, 248, 0.12), transparent 24%), linear-gradient(180deg, #020617 0%, #071220 44%, #020617 100%)'
    : 'radial-gradient(circle at 10% 16%, rgba(20, 184, 166, 0.16), transparent 26%), radial-gradient(circle at 86% 16%, rgba(59, 130, 246, 0.14), transparent 24%), linear-gradient(180deg, #f8fafc 0%, #eef4ff 44%, #f8fafc 100%)'
}))

const copy = computed(() =>
  isZh.value
    ? {
        eyebrow: '高可用 AI 中转站',
        heroTitle: '连接全球顶尖模型',
        docsLabel: '接入文档',
        serviceLabel: '客服',
        serviceTitle: '联系我们',
        announcementLabel: '公告',
        announcementTitle: '站点公告',
        announcementEmpty: '暂无公告',
        announcementEmptyDesc: '当前没有可展示的站点公告。',
        announcementRead: '已读',
        announcementUnread: '未读',
        markReadLabel: '标记已读',
        floatOne: '多账号智能路由',
        floatTwo: '客户端即配即用'
      }
    : {
        eyebrow: 'High Availability AI Gateway',
        heroTitle: 'Connect the world’s top AI models',
        docsLabel: 'Tutorial',
        serviceLabel: 'Support',
        serviceTitle: 'Contact Us',
        announcementLabel: 'Announcements',
        announcementTitle: 'Site Announcements',
        announcementEmpty: 'No announcements',
        announcementEmptyDesc: 'There are no active announcements right now.',
        announcementRead: 'Read',
        announcementUnread: 'Unread',
        markReadLabel: 'Mark as read',
        floatOne: 'Smart Multi-Account Routing',
        floatTwo: 'Ready for Official Clients'
      }
)

const heroLead = computed(() => {
  if (siteSubtitle.value && siteSubtitle.value !== DEFAULT_SITE_SUBTITLE) {
    return siteSubtitle.value
  }
  return isZh.value
    ? '一个 API Key，连接 Claude、GPT、Gemini 与 Codex / Claude Code 客户端，保持简洁、稳定、可控。'
    : 'One API key for Claude, GPT, Gemini, and official clients like Codex and Claude Code with a cleaner, steadier workflow.'
})

const primaryActionPath = computed(() => {
  if (isAuthenticated.value) return dashboardPath.value
  return registrationEnabled.value ? '/login' : '/login'
})

const primaryActionLabel = computed(() => {
  if (isAuthenticated.value) return t('home.goToDashboard')
  return isZh.value ? '立即开始' : 'Get Started'
})

const heroSignals = computed(() => [
  { label: isZh.value ? '订阅转 API' : 'Subscription to API', color: '#14b8a6' },
  { label: isZh.value ? '会话保持' : 'Sticky Session', color: '#38bdf8' },
  { label: isZh.value ? '按量计费' : 'Usage Billing', color: '#8b5cf6' }
])

const featureCards = computed(() => [
  {
    icon: 'server' as const,
    kicker: isZh.value ? 'Gateway' : 'Gateway',
    title: isZh.value ? '统一入口' : 'Unified Entry',
    description: isZh.value
      ? '一个 Key 即可接入多模型、多上游、多客户端，减少切换与重复配置。'
      : 'One key for multiple models, upstreams, and client types with less switching and less setup.',
    color: '#14b8a6'
  },
  {
    icon: 'shield' as const,
    kicker: isZh.value ? 'Routing' : 'Routing',
    title: isZh.value ? '稳定可靠' : 'High Stability',
    description: isZh.value
      ? '多账号池自动切换与智能调度，尽量把可用性留给真正的请求。'
      : 'Automatic account failover and routing keep availability focused on real traffic.',
    color: '#38bdf8'
  },
  {
    icon: 'chartBar' as const,
    kicker: isZh.value ? 'Billing' : 'Billing',
    title: isZh.value ? '消费清晰' : 'Clear Billing',
    description: isZh.value
      ? '额度、用量、客户端接入路径都保持可见，让团队更容易控制成本。'
      : 'Usage, quota, and client connection paths stay visible so cost control remains simple.',
    color: '#8b5cf6'
  }
])

const providers = computed(() => [
  {
    name: t('home.providers.claude'),
    initial: 'C',
    color: '#f97316',
    softColor: '#fb7185'
  },
  {
    name: 'GPT',
    initial: 'G',
    color: '#22c55e',
    softColor: '#14b8a6'
  },
  {
    name: t('home.providers.gemini'),
    initial: 'G',
    color: '#3b82f6',
    softColor: '#22d3ee'
  },
  {
    name: t('home.providers.antigravity'),
    initial: 'A',
    color: '#ec4899',
    softColor: '#a855f7'
  }
])

const serviceCards = computed(() => [
  {
    title: isZh.value ? '客服二维码' : 'Support QR',
    desc: isZh.value ? '扫码添加QQ解决问题' : 'Scan to add QQ for support',
    icon: 'chatBubble' as const,
    color: '#14b8a6',
    qrUrl: supportQrImageUrl.value
  }
])

marked.setOptions({
  breaks: true,
  gfm: true
})

function renderAnnouncement(content: string) {
  if (!content) return ''
  return DOMPurify.sanitize(marked.parse(content) as string)
}

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

async function openAnnouncementModal() {
  announcementModalOpen.value = true
  if (isAuthenticated.value) {
    await announcementStore.fetchAnnouncements(true)
    return
  }
  await fetchPublicAnnouncements()
}

async function markAnnouncementRead(id: number) {
  if (!isAuthenticated.value) return
  await announcementStore.markAsRead(id)
}

async function fetchPublicAnnouncements() {
  publicAnnouncementLoading.value = true
  try {
    publicAnnouncements.value = []
  } finally {
    publicAnnouncementLoading.value = false
  }
}

onMounted(async () => {
  initTheme()
  authStore.checkAuth()

  if (!appStore.publicSettingsLoaded) {
    await appStore.fetchPublicSettings()
  }

  if (isAuthenticated.value) {
    announcementStore.fetchAnnouncements()
  }
})
</script>

<style scoped>
.landing-shell {
  background:
    radial-gradient(circle at 10% 16%, rgba(20, 184, 166, 0.16), transparent 26%),
    radial-gradient(circle at 86% 16%, rgba(59, 130, 246, 0.14), transparent 24%),
    linear-gradient(180deg, #f8fafc 0%, #eef4ff 44%, #f8fafc 100%);
}

.landing-noise {
  opacity: 0.24;
  background-image:
    radial-gradient(rgba(15, 23, 42, 0.08) 0.6px, transparent 0.6px),
    radial-gradient(rgba(15, 23, 42, 0.04) 0.6px, transparent 0.6px);
  background-position: 0 0, 18px 18px;
  background-size: 36px 36px;
  mask-image: linear-gradient(180deg, rgba(0, 0, 0, 0.55), transparent 85%);
}

.landing-grid {
  opacity: 0.44;
  background-image:
    linear-gradient(rgba(148, 163, 184, 0.16) 1px, transparent 1px),
    linear-gradient(90deg, rgba(148, 163, 184, 0.16) 1px, transparent 1px);
  background-size: 72px 72px;
  mask-image: radial-gradient(circle at center, black 30%, transparent 84%);
}

.landing-spot {
  border-radius: 9999px;
  filter: blur(110px);
  opacity: 0.7;
}

.landing-spot-a {
  left: -10rem;
  top: 8rem;
  height: 18rem;
  width: 18rem;
  background: rgba(20, 184, 166, 0.2);
}

.landing-spot-b {
  right: -7rem;
  top: 11rem;
  height: 16rem;
  width: 16rem;
  background: rgba(59, 130, 246, 0.2);
}

.landing-spot-c {
  bottom: 4rem;
  left: 34%;
  height: 14rem;
  width: 14rem;
  background: rgba(168, 85, 247, 0.14);
}

.landing-display,
.brand-wordmark {
  font-family:
    'Avenir Next',
    'Segoe UI Variable Display',
    'PingFang SC',
    'Hiragino Sans GB',
    'Microsoft YaHei',
    sans-serif;
}

.studio-shell {
  position: relative;
  perspective: 1400px;
}

.studio-aura {
  background:
    radial-gradient(circle at center, rgba(20, 184, 166, 0.28), transparent 56%),
    radial-gradient(circle at 60% 48%, rgba(59, 130, 246, 0.2), transparent 54%);
}

.studio-panel {
  border: 1px solid rgba(255, 255, 255, 0.62);
  background:
    linear-gradient(145deg, rgba(255, 255, 255, 0.88), rgba(255, 255, 255, 0.58)),
    radial-gradient(circle at top, rgba(20, 184, 166, 0.14), transparent 38%);
  box-shadow:
    0 24px 90px rgba(15, 23, 42, 0.12),
    inset 0 1px 0 rgba(255, 255, 255, 0.7);
  transform: rotateX(8deg) rotateY(-9deg);
}

.studio-panel::before {
  content: '';
  position: absolute;
  inset: 0;
  pointer-events: none;
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.42), transparent 38%),
    radial-gradient(circle at 30% 20%, rgba(255, 255, 255, 0.45), transparent 24%);
}

.route-pill {
  animation: floatCard 8s ease-in-out infinite;
}

.route-pill:nth-child(2) {
  animation-delay: -1.4s;
}

.route-pill:nth-child(3) {
  animation-delay: -3s;
}

.route-pill:nth-child(4) {
  animation-delay: -4.6s;
}

.studio-core {
  isolation: isolate;
}

.core-ring,
.core-beam {
  position: absolute;
  inset: 50%;
  transform-origin: center;
}

.core-ring {
  border-radius: 9999px;
}

.core-ring-a {
  height: 7.5rem;
  width: 7.5rem;
  margin-left: -3.75rem;
  margin-top: -3.75rem;
  border: 1px solid rgba(20, 184, 166, 0.24);
  animation: pulseRing 5.4s ease-in-out infinite;
}

.core-ring-b {
  height: 9rem;
  width: 9rem;
  margin-left: -4.5rem;
  margin-top: -4.5rem;
  border: 1px solid rgba(56, 189, 248, 0.18);
  animation: pulseRing 5.4s ease-in-out infinite -2s;
}

.core-beam {
  height: 1px;
  width: 5.75rem;
  margin-left: -2.875rem;
  background: linear-gradient(90deg, transparent, rgba(20, 184, 166, 0.8), transparent);
}

.core-beam-a {
  transform: translate(-50%, -50%) rotate(0deg);
}

.core-beam-b {
  transform: translate(-50%, -50%) rotate(60deg);
}

.core-beam-c {
  transform: translate(-50%, -50%) rotate(120deg);
}

.core-node {
  animation: pulseCore 4.8s ease-in-out infinite;
}

.studio-float {
  position: absolute;
  z-index: 30;
}

.studio-float-top {
  right: -0.5rem;
  top: 4.25rem;
  animation: floatCard 6.5s ease-in-out infinite -1.5s;
}

.studio-float-bottom {
  bottom: 1rem;
  left: -1rem;
  animation: floatCard 7.2s ease-in-out infinite -3s;
}

.qr-frame {
  position: relative;
  height: 180px;
  width: 180px;
  overflow: hidden;
  border-radius: 24px;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.92), rgba(241, 245, 249, 0.72));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.75),
    0 20px 50px rgba(15, 23, 42, 0.08);
  backdrop-filter: blur(18px);
}

.announcement-body :deep(p) {
  margin-bottom: 0.8rem;
  line-height: 1.8;
}

.announcement-body :deep(a) {
  color: #0f766e;
  text-decoration: underline;
}

.announcement-body :deep(code) {
  border-radius: 0.5rem;
  background: rgba(15, 23, 42, 0.06);
  padding: 0.12rem 0.4rem;
}

.modal-fade-enter-active,
.modal-fade-leave-active {
  transition: opacity 0.16s ease, transform 0.16s ease;
}

.modal-fade-enter-from,
.modal-fade-leave-to {
  opacity: 0;
}

.modal-fade-enter-from > div,
.modal-fade-leave-to > div {
  transform: scale(0.985) translateY(4px);
}

@keyframes floatCard {
  0%,
  100% {
    transform: translateY(0);
  }
  50% {
    transform: translateY(-7px);
  }
}

@keyframes pulseRing {
  0%,
  100% {
    opacity: 0.38;
    transform: scale(0.96);
  }
  50% {
    opacity: 1;
    transform: scale(1.04);
  }
}

@keyframes pulseCore {
  0%,
  100% {
    transform: scale(1);
  }
  50% {
    transform: scale(1.04);
  }
}

.landing-shell.is-dark .landing-noise {
  opacity: 0.1;
  background-image:
    radial-gradient(rgba(255, 255, 255, 0.12) 0.6px, transparent 0.6px),
    radial-gradient(rgba(255, 255, 255, 0.06) 0.6px, transparent 0.6px);
}

.landing-shell.is-dark .landing-grid {
  opacity: 0.18;
  background-image:
    linear-gradient(rgba(56, 189, 248, 0.12) 1px, transparent 1px),
    linear-gradient(90deg, rgba(56, 189, 248, 0.12) 1px, transparent 1px);
}

.landing-shell.is-dark .studio-panel {
  border-color: rgba(255, 255, 255, 0.1);
  background:
    linear-gradient(145deg, rgba(9, 15, 27, 0.9), rgba(12, 20, 35, 0.78)),
    radial-gradient(circle at top, rgba(20, 184, 166, 0.1), transparent 38%);
  box-shadow:
    0 28px 100px rgba(0, 0, 0, 0.34),
    inset 0 1px 0 rgba(255, 255, 255, 0.08);
}

.landing-shell.is-dark .studio-panel::before {
  background:
    linear-gradient(135deg, rgba(255, 255, 255, 0.06), transparent 38%),
    radial-gradient(circle at 30% 20%, rgba(255, 255, 255, 0.06), transparent 24%);
}

.landing-shell.is-dark .studio-aura {
  background:
    radial-gradient(circle at 42% 50%, rgba(16, 185, 129, 0.2), transparent 58%),
    radial-gradient(circle at 68% 44%, rgba(56, 189, 248, 0.2), transparent 54%);
}

.landing-shell.is-dark .studio-head {
  border-color: rgba(125, 211, 252, 0.18);
  background:
    linear-gradient(135deg, rgba(15, 26, 48, 0.8), rgba(8, 17, 35, 0.74));
}

.landing-shell.is-dark .studio-stage {
  border-color: rgba(125, 211, 252, 0.14);
  background:
    linear-gradient(150deg, rgba(8, 16, 33, 0.78), rgba(5, 11, 25, 0.62));
}

.landing-shell.is-dark .route-pill {
  border-color: rgba(125, 211, 252, 0.17);
  background:
    linear-gradient(145deg, rgba(18, 34, 62, 0.78), rgba(9, 19, 40, 0.68));
  box-shadow:
    0 14px 34px rgba(2, 12, 30, 0.48),
    inset 0 1px 0 rgba(148, 210, 255, 0.08);
}

.landing-shell.is-dark .core-node {
  border-color: rgba(125, 211, 252, 0.24);
  background:
    radial-gradient(circle at 30% 24%, rgba(56, 189, 248, 0.2), transparent 58%),
    linear-gradient(145deg, rgba(15, 27, 50, 0.84), rgba(8, 18, 39, 0.76));
  box-shadow:
    0 22px 62px rgba(2, 12, 30, 0.5),
    inset 0 1px 0 rgba(148, 210, 255, 0.1);
}

.landing-shell.is-dark .studio-compat {
  border-color: rgba(125, 211, 252, 0.16);
  background:
    linear-gradient(148deg, rgba(18, 34, 62, 0.78), rgba(8, 18, 39, 0.68));
  box-shadow:
    0 18px 40px rgba(2, 10, 28, 0.42),
    inset 0 1px 0 rgba(148, 210, 255, 0.07);
}

.landing-shell.is-dark .studio-client-card {
  border-color: rgba(125, 211, 252, 0.14);
  background:
    linear-gradient(148deg, rgba(17, 31, 57, 0.74), rgba(8, 18, 39, 0.64));
  color: rgba(186, 211, 242, 0.92);
  box-shadow:
    0 14px 30px rgba(2, 10, 24, 0.38),
    inset 0 1px 0 rgba(148, 210, 255, 0.06);
}

.landing-shell.is-dark .studio-signal-card {
  border-color: rgba(125, 211, 252, 0.14);
  background:
    linear-gradient(145deg, rgba(14, 28, 54, 0.74), rgba(8, 18, 38, 0.62));
  box-shadow:
    0 12px 28px rgba(2, 10, 24, 0.38),
    inset 0 1px 0 rgba(148, 210, 255, 0.05);
}

.landing-shell.is-dark .studio-float {
  border-color: rgba(125, 211, 252, 0.18);
  background:
    linear-gradient(148deg, rgba(21, 39, 68, 0.78), rgba(10, 22, 45, 0.7));
  box-shadow:
    0 18px 42px rgba(2, 12, 30, 0.48),
    inset 0 1px 0 rgba(148, 210, 255, 0.08);
}

.landing-shell.is-dark .qr-frame {
  background:
    linear-gradient(180deg, rgba(15, 23, 42, 0.88), rgba(15, 23, 42, 0.66));
  box-shadow:
    inset 0 1px 0 rgba(255, 255, 255, 0.06),
    0 24px 54px rgba(0, 0, 0, 0.32);
}

.landing-shell.is-dark .announcement-body :deep(a) {
  color: #67e8f9;
}

.landing-shell.is-dark .announcement-body :deep(code) {
  background: rgba(255, 255, 255, 0.08);
}

.site-logo-light {
  filter: none;
}

.site-logo-dark {
  filter: brightness(1.15) contrast(1.08) saturate(1.08);
}

@media (max-width: 1024px) {
  .studio-panel {
    transform: none;
  }

  .studio-float-top {
    right: 0.5rem;
  }

  .studio-float-bottom {
    left: 0.5rem;
  }
}

@media (max-width: 640px) {
  .landing-grid {
    background-size: 48px 48px;
  }

  .studio-float {
    position: static;
    margin-top: 0.75rem;
    display: inline-flex;
  }
}
</style>
