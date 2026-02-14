---
title: Project Management
sidebar_position: 4
---

# Project Management in Bowrain

Bowrain provides a store-backed project system for managing translation content with versioning and connector integration.

## Content Store

Projects in Bowrain are backed by the Content Store, a local SQLite database that provides:

- Persistent block storage with content-addressable deduplication
- Version snapshots for tracking changes over time
- KAZ export/import for sharing projects

## Creating a Store-Backed Project

1. Open the Projects view
2. Click **Create Project**
3. Set the project name, source locale, and target locales
4. The project is created in the Content Store

## Version History

Create version snapshots to track your project's state over time:

1. Open a project
2. Navigate to the version history section
3. Click **Create Version** and provide a label
4. View past versions and the changes between them

## Connector Integration

Store-backed projects work seamlessly with connectors:

1. **Pull** content from a connector into your project
2. Translate using the translation editor, TM, or AI
3. **Push** translations back to the connector

## Import and Export

Export your project as a KAZ archive to share it or back it up:

- **Export**: Creates a portable `.kaz` file with all blocks, translations, and metadata
- **Import**: Opens a `.kaz` file and creates a new project in the store
