package com.cristim.dailyprogress

import android.app.Application
import com.cristim.dailyprogress.core.CoreClient
import com.cristim.dailyprogress.core.CoreRepository

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
class AppContainer(app: android.content.Context) {
    val coreClient: CoreClient = CoreClient(app)
    val coreRepository: CoreRepository = CoreRepository(coreClient)
}
