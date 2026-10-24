# dw

dw is a place to drop topics you want to think about, find them again later, and keep working on them in the same spot. Every topic is an ordinary directory, usually inside an Obsidian vault.

## Language

**Root**:
The single directory whose immediate subdirectories are the topics.
_Avoid_: vault, workspace root

**Topic**:
A directory directly under the root that holds everything about one subject, whether it's a throwaway note or a long-running project.
_Avoid_: workspace, project, discussion

**Topic name**:
The topic's directory name with the leading `YYYY-MM-DD-` creation date removed, written exactly as the user typed it. Topic names are unique within the root.
_Avoid_: slug, title

**Main note**:
The Markdown file inside a topic that has the same name as the topic (`<topic name>.md`), and the entry point to it.
_Avoid_: STATE.md, README, index

**Last visited**:
The topic most recently opened through dw, pinned to the top of the picker.
_Avoid_: current topic, active topic
