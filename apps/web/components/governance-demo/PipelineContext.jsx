import {useDemo} from './DemoState';
import {useWorkspace} from './WorkspaceContext';
import {seedWorkspacePipeline} from './workspace-seeds.mjs';
export function usePipeline(){const {workspace}=useWorkspace();return useDemo('pipeline-state',seedWorkspacePipeline(workspace))}
