package com.cristim.dailyprogress.core

/**
 * Abstraction over the CoreRepository daily-prompt calls, decoupled from the
 * concrete CoreRepository so consumers are testable without a real
 * mobilecore.Core handle.
 *
 * CoreRepository implements this interface; tests supply a fake.
 */
interface DailyPromptOps {
    /** Current daily prompt text; empty string when unset. */
    suspend fun dailyPrompt(): String

    /** Persists [text] as the new daily prompt (trimmed by Core). */
    suspend fun setDailyPrompt(text: String)
}
