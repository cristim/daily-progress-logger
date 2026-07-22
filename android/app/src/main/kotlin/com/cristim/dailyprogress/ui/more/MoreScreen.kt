package com.cristim.dailyprogress.ui.more

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Repeat
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Sync
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.ListItem
import androidx.compose.material3.ListItemDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector

/**
 * "More" bottom-nav destination: navigation menu for secondary screens.
 * Items are disabled placeholders until their respective phases land.
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MoreScreen(onRecurringClick: () -> Unit = {}) {
    Scaffold(
        topBar = { TopAppBar(title = { Text("More") }) },
    ) { padding ->
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding),
        ) {
            item {
                MoreMenuItem(
                    icon = Icons.Filled.Folder,
                    label = "Projects",
                    enabled = false,
                )
            }
            item {
                MoreMenuItem(
                    icon = Icons.Filled.Repeat,
                    label = "Recurring Templates",
                    enabled = true,
                    onClick = onRecurringClick,
                )
            }
            item {
                MoreMenuItem(
                    icon = Icons.Filled.Delete,
                    label = "Recycle Bin",
                    enabled = false,
                )
            }
            item {
                MoreMenuItem(
                    icon = Icons.Filled.Sync,
                    label = "Sync & Account",
                    enabled = false,
                )
            }
            item {
                MoreMenuItem(
                    icon = Icons.Filled.Settings,
                    label = "Settings",
                    enabled = false,
                )
            }
        }
    }
}

@Composable
private fun MoreMenuItem(
    icon: ImageVector,
    label: String,
    enabled: Boolean,
    onClick: () -> Unit = {},
) {
    val contentColor = if (enabled) {
        MaterialTheme.colorScheme.onSurface
    } else {
        MaterialTheme.colorScheme.onSurface.copy(alpha = 0.38f)
    }

    ListItem(
        headlineContent = {
            Text(
                text = label,
                color = contentColor,
                modifier = Modifier.fillMaxWidth(),
            )
        },
        leadingContent = {
            Icon(
                imageVector = icon,
                contentDescription = null,
                tint = contentColor,
            )
        },
        modifier = if (enabled) Modifier.clickable(onClick = onClick) else Modifier,
        colors = ListItemDefaults.colors(),
    )
}
