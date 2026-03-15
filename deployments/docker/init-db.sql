-- Crear bases de datos para Heimdall (solo en primer arranque del volumen)
CREATE DATABASE heimdall_datasets;
CREATE DATABASE heimdall_scope_service;
CREATE DATABASE heimdall_routing_service;
CREATE DATABASE heimdall_target_service;
CREATE DATABASE heimdall_campaign_service;

-- Bases de datos de test (aisladas; los tests usan estas para no tocar datos de desarrollo)
CREATE DATABASE heimdall_datasets_test;
CREATE DATABASE heimdall_scope_service_test;
CREATE DATABASE heimdall_routing_service_test;
CREATE DATABASE heimdall_target_service_test;
CREATE DATABASE heimdall_campaign_service_test;
