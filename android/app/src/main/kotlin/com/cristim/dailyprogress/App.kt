package com.cristim.dailyprogress

import android.app.Application
import android.content.Context
import com.cristim.dailyprogress.core.CoreClient
import com.cristim.dailyprogress.core.CoreRepository
import kotlinx.coroutines.flow.MutableStateFlow

/**
 * Application subclass that owns the dependency graph.
 * Uses manual DI (a single [AppContainer]) — no Hilt in v1.
 */
class App : Application() {

    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(applicationContext)
    }
}

/**
 * Manually-wired dependency graph.
 * Singletons are shared across the process lifetime; the [CoreClient] and
 * [CoreRepository] own the gomobile Core handle and the single-threaded IO
 * dispatcher respectively.
 */
class AppContainer(app: Context) {
    val coreClient: CoreClient = CoreClient(app)
    val coreRepository: CoreRepository = CoreRepository(coreClient)

    /**
     * Cross-screen invalidation counter. Every screen that mutates shared data
     * increments this after a successful call; sibling screens observe it and
     * refresh. Avoids polling and keeps reads lazy.
     */
    val dataVersion: MutableStateFlow<Int> = MutableStateFlow(0)
}
