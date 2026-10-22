SELECT c.id,c.workspace_id,c.dataset_version_id,n.provider_instance_id,n.incarnation_id,
		       n.namespace,c.object_key,c.provider_version,c.content_sha256,c.byte_size
		FROM lifecycle_fixture_copy c JOIN lifecycle_fixture_namespace n ON n.id=c.namespace_id
		WHERE c.workspace_id=$1 AND c.id=ANY($2) ORDER BY c.id
	