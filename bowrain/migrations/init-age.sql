-- Initialize Apache AGE extension and graph
CREATE EXTENSION IF NOT EXISTS age;
LOAD 'age';
SET search_path = ag_catalog, "$user", public;

-- Create the default graph
SELECT create_graph('bowrain_graph');
