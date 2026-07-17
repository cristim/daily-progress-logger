package com.cristim.dailyprogress.core

/**
 * Typed error hierarchy for errors returned by Core methods.
 *
 * gomobile flattens Go errors to message strings; the Core contract guarantees
 * every error message starts with a stable code prefix followed by ": ".
 * Hosts detect error kinds by prefix-matching (errors.Is is unavailable across
 * the gomobile boundary).
 *
 * Maps directly to mobilecore/dto.go ErrCode* constants.
 */
sealed class CoreError(val raw: String) : Exception(raw) {
    /** CAS_MISMATCH: tree is stale; call TreeJSON and re-present the action. */
    class CasMismatch(raw: String) : CoreError(raw)

    /** NOT_FOUND: referenced item (project, backlog entry, etc.) does not exist. */
    class NotFound(raw: String) : CoreError(raw)

    /** BAD_INPUT: invalid argument (bad state string, bad date, bad choice, etc.). */
    class BadInput(raw: String) : CoreError(raw)

    /** SYNC_AUTH: OAuth token is invalid or expired; host must re-authenticate. */
    class SyncAuth(raw: String) : CoreError(raw)

    /** INTERNAL: unexpected internal error (bug). */
    class Internal(raw: String) : CoreError(raw)

    /** Unknown: error message did not carry a recognised code. */
    class Unknown(raw: String) : CoreError(raw)

    /**
     * ContractViolation: the app received a valid response from Core but could not
     * deserialise it into the expected DTO. Indicates contract drift between the app
     * and the Core version it was compiled against. Distinct from [Unknown], which
     * is a Core-returned error with an unrecognised code prefix.
     */
    class ContractViolation(raw: String) : CoreError(raw)

    companion object {
        private const val CAS_MISMATCH_PREFIX = "CAS_MISMATCH"
        private const val NOT_FOUND_PREFIX = "NOT_FOUND"
        private const val BAD_INPUT_PREFIX = "BAD_INPUT"
        private const val SYNC_AUTH_PREFIX = "SYNC_AUTH"
        private const val INTERNAL_PREFIX = "INTERNAL"

        /**
         * Parses a Core error message into a typed [CoreError].
         * Falls back to [Unknown] when the message does not carry a known prefix.
         */
        fun parse(msg: String): CoreError = when {
            msg.startsWith("$CAS_MISMATCH_PREFIX: ") -> CasMismatch(msg)
            msg.startsWith("$NOT_FOUND_PREFIX: ")    -> NotFound(msg)
            msg.startsWith("$BAD_INPUT_PREFIX: ")    -> BadInput(msg)
            msg.startsWith("$SYNC_AUTH_PREFIX: ")    -> SyncAuth(msg)
            msg.startsWith("$INTERNAL_PREFIX: ")     -> Internal(msg)
            else                                     -> Unknown(msg)
        }

        /** Parse from a Throwable; handles null message gracefully. */
        fun parse(t: Throwable): CoreError = parse(t.message.orEmpty())
    }
}
