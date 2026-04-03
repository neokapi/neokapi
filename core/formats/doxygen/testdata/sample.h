// Not a Doxygen comment
#include <stdio.h>

/// \brief Brief description of the class.
///
/// A more detailed class description
/// that spans multiple lines.
///
/// \code
///     jimmy.crack("corn");
/// \endcode
///
/// After the code block.
class Test {
public:
    /// Constructor description.
    Test();

    int x; ///< A trailing comment

    /**
     * A Javadoc method description.
     * \param name The name parameter
     * \return The result value
     */
    int doSomething(const char* name);
};
